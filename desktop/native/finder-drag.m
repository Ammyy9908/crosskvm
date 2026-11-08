#import <AppKit/AppKit.h>
#import <ApplicationServices/ApplicationServices.h>
#include <node_api.h>
static napi_value readDrag(napi_env env, napi_callback_info info) {
 @autoreleasepool {
  napi_value result,value,files;
  napi_create_object(env,&result);
  NSPasteboard *board = [NSPasteboard pasteboardWithName:NSPasteboardNameDrag];
  napi_create_int64(env,board.changeCount,&value);napi_set_named_property(env,result,"change",value);
  BOOL down=CGEventSourceButtonState(kCGEventSourceStateHIDSystemState,kCGMouseButtonLeft);
  napi_get_boolean(env,down,&value);napi_set_named_property(env,result,"down",value);
  napi_create_array(env,&files);napi_set_named_property(env,result,"files",files);
  if (!down || ![[NSWorkspace sharedWorkspace].frontmostApplication.bundleIdentifier isEqualToString:@"com.apple.finder"]) return result;
  NSArray<NSURL *> *urls = [board readObjectsForClasses:@[[NSURL class]]
    options:@{NSPasteboardURLReadingFileURLsOnlyKey:@YES}];
  uint32_t index=0;
  for (NSURL *url in urls) {
   if (!url.isFileURL || index >= 100) continue;
   napi_value file; napi_create_string_utf8(env,url.path.UTF8String,NAPI_AUTO_LENGTH,&file);
   napi_set_element(env,files,index++,file);
  }
  // Finder may expose its legacy file-list representation during a drag.
  if (index == 0) {
   id names = [board propertyListForType:@"NSFilenamesPboardType"];
   if ([names isKindOfClass:[NSArray class]]) {
    for (id name in names) {
     if (![name isKindOfClass:[NSString class]] || ![name isAbsolutePath] || index >= 100) continue;
     napi_value file; napi_create_string_utf8(env,[name UTF8String],NAPI_AUTO_LENGTH,&file);
     napi_set_element(env,files,index++,file);
    }
   }
  }
  return result;
 }
}
static napi_value cancelFinder(napi_env env,napi_callback_info info) {
 @autoreleasepool {
  // End Finder's local drag without authorizing a move or local drop.
  if ([[NSWorkspace sharedWorkspace].frontmostApplication.bundleIdentifier isEqualToString:@"com.apple.finder"]) {
   for (int down=1; down>=0; down--) {
    CGEventRef e=CGEventCreateKeyboardEvent(NULL,53,down);
    CGEventSetFlags(e,0);
    CGEventSetIntegerValueField(e,kCGEventSourceUserData,0x584B564D);
    CGEventPost(kCGHIDEventTap,e); CFRelease(e);
   }
  }
  napi_value value;napi_get_undefined(env,&value);return value;
 }
}
static napi_value init(napi_env env,napi_value exports) {
 napi_value fn;napi_create_function(env,"readDrag",NAPI_AUTO_LENGTH,readDrag,NULL,&fn);
 napi_set_named_property(env,exports,"readDrag",fn);
 napi_create_function(env,"cancelFinder",NAPI_AUTO_LENGTH,cancelFinder,NULL,&fn);
 napi_set_named_property(env,exports,"cancelFinder",fn);return exports;
}
NAPI_MODULE(NODE_GYP_MODULE_NAME,init)
