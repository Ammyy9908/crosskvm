#include <node_api.h>
#include <ApplicationServices/ApplicationServices.h>

static bool hidden = false;

static napi_value setRemote(napi_env env, napi_callback_info info) {
    size_t argc = 1;
    napi_value arg;
    bool remote = false;
    napi_get_cb_info(env, info, &argc, &arg, NULL, NULL);
    if (argc != 1 || napi_get_value_bool(env, arg, &remote) != napi_ok) {
        napi_throw_type_error(env, NULL, "setRemote expects a boolean");
        return NULL;
    }
    if (remote != hidden) {
        CGError result;
        if (remote) {
            result = CGDisplayHideCursor(kCGNullDirectDisplay);
            if (result == kCGErrorSuccess) {
                result = CGAssociateMouseAndMouseCursorPosition(false);
                if (result != kCGErrorSuccess) CGDisplayShowCursor(kCGNullDirectDisplay);
            }
        } else {
            result = CGAssociateMouseAndMouseCursorPosition(true);
            CGError shown = CGDisplayShowCursor(kCGNullDirectDisplay);
            if (result == kCGErrorSuccess) result = shown;
        }
        if (result != kCGErrorSuccess) {
            napi_throw_error(env, NULL, "macOS cursor control failed");
            return NULL;
        }
        hidden = remote;
    }
    napi_value output;
    napi_get_undefined(env, &output);
    return output;
}

static napi_value init(napi_env env, napi_value exports) {
    napi_value fn;
    napi_create_function(env, "setRemote", NAPI_AUTO_LENGTH, setRemote, NULL, &fn);
    napi_set_named_property(env, exports, "setRemote", fn);
    return exports;
}
NAPI_MODULE(NODE_GYP_MODULE_NAME, init)
