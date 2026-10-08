#import <AppKit/AppKit.h>
#import <ImageIO/ImageIO.h>
#include <node_api.h>

// Only inline image data is read. File URLs are never opened.
static napi_value readPNG(napi_env env, napi_callback_info info) {
 @autoreleasepool {
  NSPasteboard *pb = [NSPasteboard generalPasteboard];
  NSString *type = [pb availableTypeFromArray:@[NSPasteboardTypePNG, NSPasteboardTypeTIFF, @"public.jpeg"]];
  napi_value result;
  if (!type) { napi_get_null(env, &result); return result; }
  NSData *data = [pb dataForType:type];
  if (!data || data.length > 64 * 1024 * 1024) {
   napi_throw_error(env, NULL, "Image not shared: clipboard image is too large or unavailable."); return NULL;
  }
  CGImageSourceRef source = CGImageSourceCreateWithData((__bridge CFDataRef)data, NULL);
  if (!source) { napi_throw_error(env, NULL, "Image not shared: unsupported image data."); return NULL; }
  NSDictionary *properties = CFBridgingRelease(CGImageSourceCopyPropertiesAtIndex(source, 0, NULL));
  double width = [properties[(__bridge NSString *)kCGImagePropertyPixelWidth] doubleValue];
  double height = [properties[(__bridge NSString *)kCGImagePropertyPixelHeight] doubleValue];
  if (width <= 0 || height <= 0 || width * height > 16000000) {
   CFRelease(source); napi_throw_error(env, NULL, "Image not shared: maximum size is 16 megapixels."); return NULL;
  }
  CGImageRef image = CGImageSourceCreateImageAtIndex(source, 0, NULL);
  CFRelease(source);
  if (!image) { napi_throw_error(env, NULL, "Image not shared: image decoding failed."); return NULL; }
  NSBitmapImageRep *rep = [[NSBitmapImageRep alloc] initWithCGImage:image];
  CGImageRelease(image);
  NSData *png = [rep representationUsingType:NSBitmapImageFileTypePNG properties:@{}];
  if (!png || png.length > 512 * 1024) {
   napi_throw_error(env, NULL, "Image not shared: converted PNG exceeds 512 KiB."); return NULL;
  }
  napi_create_buffer_copy(env, png.length, png.bytes, NULL, &result);
  return result;
 }
}
static napi_value init(napi_env env, napi_value exports) {
 napi_value fn;
 napi_create_function(env, "readPNG", NAPI_AUTO_LENGTH, readPNG, NULL, &fn);
 napi_set_named_property(env, exports, "readPNG", fn);
 return exports;
}
NAPI_MODULE(NODE_GYP_MODULE_NAME, init)
