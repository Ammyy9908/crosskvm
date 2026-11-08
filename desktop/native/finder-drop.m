#import <AppKit/AppKit.h>
#import <ApplicationServices/ApplicationServices.h>
#include <node_api.h>
#define MARKER 0x584B564D
static NSPanel *panel;
static NSArray *paths;
static NSUInteger generation=0,completed=0;
static BOOL started=NO;
static BOOL allowRelease=NO;
static NSString *failure=@"";
static int state=0; // idle, starting, ready, copied, cancelled, failed
static CFMachPortRef tap;
static CFRunLoopSourceRef tapSource;
static void mouse(CGEventType type) {
 CGEventRef current=CGEventCreate(NULL);
 CGPoint point=CGEventGetLocation(current);CFRelease(current);
 CGEventRef event=CGEventCreateMouseEvent(NULL,type,point,kCGMouseButtonLeft);
 CGEventSetFlags(event,0);CGEventSetIntegerValueField(event,kCGEventSourceUserData,MARKER);
 CGEventPost(kCGHIDEventTap,event);CFRelease(event);
}
static void cleanup(void) {
 if(tapSource){CFRunLoopRemoveSource(CFRunLoopGetMain(),tapSource,kCFRunLoopCommonModes);CFRelease(tapSource);tapSource=NULL;}
 if(tap){CGEventTapEnable(tap,false);CFRelease(tap);tap=NULL;}
 [panel orderOut:nil];panel=nil;
}
static CGEventRef convertMove(CGEventTapProxy proxy,CGEventType type,CGEventRef event,void *context) {
 if(type==kCGEventTapDisabledByTimeout||type==kCGEventTapDisabledByUserInput){if(tap)CGEventTapEnable(tap,true);return event;}
 // Only the explicit drag_release packet may end a remote drag. An injected
 // mouse-up can reach the event stream before that packet is handled.
 if((state==1||state==2) && !allowRelease && type==kCGEventLeftMouseUp && CGEventGetIntegerValueField(event,kCGEventSourceUserData)==MARKER)return NULL;
 if(state==2 && type==kCGEventMouseMoved && CGEventGetIntegerValueField(event,kCGEventSourceUserData)==MARKER){
  CGEventSetType(event,kCGEventLeftMouseDragged);
  CGEventSetIntegerValueField(event,kCGMouseEventButtonNumber,kCGMouseButtonLeft);
 }
 return event;
}
static void cancelPromises(void);
@interface CrossKVMFilePromise : NSObject <NSFilePromiseProviderDelegate>
@property NSString *name;
@property long long size;
@property NSUInteger generation;
@property NSCondition *condition;
@property NSString *receivedPath;
@property NSError *error;
@end
@implementation CrossKVMFilePromise
-(instancetype)init {if((self=[super init]))_condition=[NSCondition new];return self;}
-(NSString *)filePromiseProvider:(NSFilePromiseProvider *)provider fileNameForType:(NSString *)type {return self.name;}
-(NSOperationQueue *)operationQueueForFilePromiseProvider:(NSFilePromiseProvider *)provider {
 static NSOperationQueue *queue;static dispatch_once_t once;
 dispatch_once(&once,^{queue=[NSOperationQueue new];queue.maxConcurrentOperationCount=4;});return queue;
}
-(void)filePromiseProvider:(NSFilePromiseProvider *)provider writePromiseToURL:(NSURL *)url completionHandler:(void (^)(NSError *))completionHandler {
 [self.condition lock];NSDate *deadline=[NSDate dateWithTimeIntervalSinceNow:1800];
 while(!self.receivedPath&&!self.error){if(![self.condition waitUntilDate:deadline]){self.error=[NSError errorWithDomain:@"CrossKVM" code:1 userInfo:@{NSLocalizedDescriptionKey:@"File transfer timed out"}];break;}}
 NSString *source=self.receivedPath;NSError *error=self.error;[self.condition unlock];
 if(!error){
  __block NSError *copyError=nil;NSError *coordinationError=nil;
  NSFileCoordinator *coordinator=[[NSFileCoordinator alloc] initWithFilePresenter:nil];
  [coordinator coordinateWritingItemAtURL:url options:0 error:&coordinationError byAccessor:^(NSURL *destination){
   [[NSFileManager defaultManager] copyItemAtURL:[NSURL fileURLWithPath:source] toURL:destination error:&copyError];
  }];error=coordinationError?:copyError;
 }
 completionHandler(error);
 dispatch_async(dispatch_get_main_queue(),^{
  if(self.generation!=generation)return;
  if(error){failure=error.localizedDescription;state=5;cancelPromises();}
  else if(++completed==paths.count && state!=4 && state!=5)state=3;
 });
}
@end
static void cancelPromises(void){
 for(CrossKVMFilePromise *file in paths){[file.condition lock];file.error=[NSError errorWithDomain:@"CrossKVM" code:2 userInfo:@{NSLocalizedDescriptionKey:@"Transfer cancelled or disconnected"}];[file.condition broadcast];[file.condition unlock];}
}
@interface CrossKVMDragView : NSView <NSDraggingSource>
@end
@implementation CrossKVMDragView
-(BOOL)acceptsFirstMouse:(NSEvent *)event {return YES;}
-(void)mouseDown:(NSEvent *)event {
 if(state!=1)return;
 NSMutableArray *items=[NSMutableArray array];
 for(CrossKVMFilePromise *file in paths){
  NSFilePromiseProvider *provider=[[NSFilePromiseProvider alloc] initWithFileType:@"public.data" delegate:file];
  NSDraggingItem *item=[[NSDraggingItem alloc] initWithPasteboardWriter:provider];
  NSImage *icon=[NSImage imageNamed:NSImageNameMultipleDocuments];icon.size=NSMakeSize(32,32);
  [item setDraggingFrame:NSMakeRect(0,0,32,32) contents:icon];[items addObject:item];
 }
 @try {
  NSDraggingSession *session=[self beginDraggingSessionWithItems:items event:event source:self];
  if(!session){failure=@"AppKit returned no dragging session";state=5;mouse(kCGEventLeftMouseUp);cleanup();return;}
  session.animatesToStartingPositionsOnCancelOrFail=NO;
 } @catch(NSException *exception) {
  failure=[NSString stringWithFormat:@"AppKit drag rejected: %@",exception.reason];
  state=5;mouse(kCGEventLeftMouseUp);cleanup();
 }
}
-(NSDragOperation)draggingSession:(NSDraggingSession *)session sourceOperationMaskForDraggingContext:(NSDraggingContext)context {return NSDragOperationCopy;}
-(BOOL)ignoreModifierKeysForDraggingSession:(NSDraggingSession *)session {return YES;}
-(void)draggingSession:(NSDraggingSession *)session willBeginAtPoint:(NSPoint)point {started=YES;state=2;panel.ignoresMouseEvents=YES;}
-(void)draggingSession:(NSDraggingSession *)session endedAtPoint:(NSPoint)point operation:(NSDragOperation)operation {
 state=(operation & NSDragOperationCopy)?(completed==paths.count?3:6):4;
 if(state==4){failure=@"Native drag cancelled or destination did not accept files";cancelPromises();}
 // Release the synthetic button even when Escape ended the native session.
 mouse(kCGEventLeftMouseUp);cleanup();
}
@end
static napi_value begin(napi_env env,napi_callback_info info) {
 @autoreleasepool {
  size_t argc=1;napi_value arg;bool array=false;uint32_t length=0;
  napi_get_cb_info(env,info,&argc,&arg,NULL,NULL);
  if(argc!=1||napi_is_array(env,arg,&array)!=napi_ok||!array||napi_get_array_length(env,arg,&length)!=napi_ok||length==0||length>100){napi_throw_type_error(env,NULL,"Expected staged file paths");return NULL;}
  if(state==1||state==2||state==6){napi_throw_error(env,NULL,"A Finder drag is already active");return NULL;}
  NSMutableArray *files=[NSMutableArray array];
  for(uint32_t i=0;i<length;i++){
   napi_value item,name,size;size_t bytes=0;double count=0;napi_get_element(env,arg,i,&item);
   if(napi_get_named_property(env,item,"name",&name)!=napi_ok||napi_get_value_string_utf8(env,name,NULL,0,&bytes)!=napi_ok||bytes>1024||napi_get_named_property(env,item,"size",&size)!=napi_ok||napi_get_value_double(env,size,&count)!=napi_ok||count<0||count>1073741824){napi_throw_type_error(env,NULL,"Invalid file promise");return NULL;}
   char *buffer=calloc(bytes+1,1);napi_get_value_string_utf8(env,name,buffer,bytes+1,&bytes);
   NSString *filename=[[NSString alloc] initWithBytes:buffer length:bytes encoding:NSUTF8StringEncoding];free(buffer);
   if(!filename.length||![filename.lastPathComponent isEqualToString:filename]){napi_throw_error(env,NULL,"Invalid promised name");return NULL;}
   CrossKVMFilePromise *file=[CrossKVMFilePromise new];file.name=filename;file.size=(long long)count;file.generation=generation+1;[files addObject:file];
  }
  generation++;completed=0;
  tap=CGEventTapCreate(kCGSessionEventTap,kCGHeadInsertEventTap,kCGEventTapOptionDefault,(CGEventMaskBit(kCGEventMouseMoved)|CGEventMaskBit(kCGEventLeftMouseUp)),convertMove,NULL);
  if(!tap){napi_throw_error(env,NULL,"Accessibility access is required for Finder dragging");return NULL;}
  tapSource=CFMachPortCreateRunLoopSource(NULL,tap,0);CFRunLoopAddSource(CFRunLoopGetMain(),tapSource,kCFRunLoopCommonModes);CGEventTapEnable(tap,true);
  paths=files;state=1;started=NO;allowRelease=NO;failure=@"";NSPoint point=NSEvent.mouseLocation;
  panel=[[NSPanel alloc] initWithContentRect:NSMakeRect(point.x-16,point.y-16,32,32) styleMask:NSWindowStyleMaskBorderless|NSWindowStyleMaskNonactivatingPanel backing:NSBackingStoreBuffered defer:NO];
  panel.level=NSStatusWindowLevel;panel.opaque=NO;panel.backgroundColor=NSColor.clearColor;panel.alphaValue=0.02;
  panel.hidesOnDeactivate=NO;panel.collectionBehavior=NSWindowCollectionBehaviorCanJoinAllSpaces|NSWindowCollectionBehaviorFullScreenAuxiliary;
  panel.contentView=[[CrossKVMDragView alloc] initWithFrame:NSMakeRect(0,0,32,32)];
  [panel orderFrontRegardless];
  // Bind startup to our source view explicitly. A hit-tested system click can
  // miss a transparent panel or race a remote movement onto another window.
  NSEvent *event=[NSEvent mouseEventWithType:NSEventTypeLeftMouseDown location:NSMakePoint(16,16)
   modifierFlags:0 timestamp:NSProcessInfo.processInfo.systemUptime windowNumber:panel.windowNumber
   context:nil eventNumber:0 clickCount:1 pressure:1.0];
  [(CrossKVMDragView *)panel.contentView mouseDown:event];
  napi_value result;napi_get_undefined(env,&result);return result;
 }
}
static napi_value status(napi_env env,napi_callback_info info){
 napi_value result,value;napi_create_object(env,&result);
 napi_create_int32(env,state,&value);napi_set_named_property(env,result,"state",value);
 napi_get_boolean(env,started,&value);napi_set_named_property(env,result,"started",value);
 napi_create_string_utf8(env,failure.UTF8String,NAPI_AUTO_LENGTH,&value);napi_set_named_property(env,result,"error",value);
 return result;
}
static napi_value drop(napi_env env,napi_callback_info info){if(state==2){allowRelease=YES;mouse(kCGEventLeftMouseUp);}napi_value result;napi_get_undefined(env,&result);return result;}
static napi_value cancel(napi_env env,napi_callback_info info){
 if(state==1||state==2||state==6){
  BOOL dragging=(state==1||state==2);
  cancelPromises();
  allowRelease=YES;failure=@"Drag cancelled by sender, disconnect or Stop KVM";
  if(dragging)for(int down=1;down>=0;down--){CGEventRef e=CGEventCreateKeyboardEvent(NULL,53,down);CGEventSetFlags(e,0);CGEventSetIntegerValueField(e,kCGEventSourceUserData,MARKER);CGEventPost(kCGHIDEventTap,e);CFRelease(e);}
  if(dragging)mouse(kCGEventLeftMouseUp);state=4;cleanup();
 }
 napi_value result;napi_get_undefined(env,&result);return result;
}
static napi_value provide(napi_env env,napi_callback_info info){
 size_t argc=2;napi_value args[2];uint32_t index;size_t length;
 napi_get_cb_info(env,info,&argc,args,NULL,NULL);
 if(argc!=2||napi_get_value_uint32(env,args[0],&index)!=napi_ok||index>=paths.count||napi_get_value_string_utf8(env,args[1],NULL,0,&length)!=napi_ok||length>32768){napi_throw_error(env,NULL,"Invalid delivered file");return NULL;}
 char *buffer=calloc(length+1,1);napi_get_value_string_utf8(env,args[1],buffer,length+1,&length);
 NSString *file=[[NSString alloc] initWithBytes:buffer length:length encoding:NSUTF8StringEncoding];free(buffer);
 CrossKVMFilePromise *promise=paths[index];
 NSDictionary *attributes=[[NSFileManager defaultManager] attributesOfItemAtPath:file error:nil];
 if(!file.isAbsolutePath||![attributes[NSFileType] isEqualToString:NSFileTypeRegular]||[attributes[NSFileSize] longLongValue]!=promise.size){napi_throw_error(env,NULL,"Delivered file does not match promise");return NULL;}
 [promise.condition lock];promise.receivedPath=file;[promise.condition broadcast];[promise.condition unlock];
 napi_value result;napi_get_undefined(env,&result);return result;
}
static napi_value init(napi_env env,napi_value exports){
 napi_property_descriptor props[]={
 {"provide",NULL,provide,NULL,NULL,NULL,napi_default,NULL},{"begin",NULL,begin,NULL,NULL,NULL,napi_default,NULL},{"status",NULL,status,NULL,NULL,NULL,napi_default,NULL},
 {"drop",NULL,drop,NULL,NULL,NULL,napi_default,NULL},{"cancel",NULL,cancel,NULL,NULL,NULL,napi_default,NULL}};
 napi_define_properties(env,exports,5,props);return exports;
}
NAPI_MODULE(NODE_GYP_MODULE_NAME,init)
