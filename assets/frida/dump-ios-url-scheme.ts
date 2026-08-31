/*
Inspired by: https://codeshare.frida.re/@DuffyAPP-IT/nsurl--ios13/

Help:
https://frida.re/docs/javascript-api/
https://github.com/noobpk/frida-ios-hook/blob/master/frida-ios-hook/frida-scripts/dump-ios-url-scheme.js
*/

import ObjC from "frida-objc-bridge";

var className = "NSURLSession";
var funcName = "- dataTaskWithRequest:completionHandler:";


var hook = eval('ObjC.classes.' + className + '["' + funcName + '"]');

if (ObjC.available) { // Checks for ObjC codebase
    //console.log("[FS] Finding domains");
    var list = []
    // Creates the interceptor api
    Interceptor.attach(hook.implementation, {
        // ObjectiveC method arguments:
        // 0. 'self'
        // 1. The selector (NSURLSession)
        // 2. The first argument to the funcName selector
        onEnter: function (args) {
            var request = new ObjC.Object(args[2])
            // console.log('REQUEST TYPE: ' + request.HTTPMethod());
            // console.log('HTTPHeaders: ', request.HTTPMethod())
            // console.log('[F] URL: ' + request.URL().toString().slice(0,50))
            //console.log('[F] Header: ' + request.allHTTPHeaderFields())
            if (request.attribution() != 0) {
                console.log('[FS] Request ->' + request.attribution())
            }
            var url = request.URL().toString()
            list.push(url)
        }
        send(urlg)
    })

} else {
    console.log("[F] ERROR: Objective-C runtime is not available.")
};



