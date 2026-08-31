import ObjC from "frida-objc-bridge";

// console.log("[*] Script loaded successfully.");

// Original implementation, enumerated every runtime class (system frameworks +
// jailbreak tweak dylibs included), which crashed the target process (SIGSEGV)
// a few seconds later on jailbroken devices when the count reached ~68856.
function run_show_classes_of_app() {
    var count = 0
    var list = []
    for (var className in ObjC.classes) {
        if (ObjC.classes.hasOwnProperty(className)) {
            list.push(className)
            count = count + 1
        }
    }
    send(list)
};

// Only classes owned by modules inside the app's own bundle directory are collected
// (the main executable plus any embedded/third-party frameworks used for SDK detection).
// System frameworks and injected jailbreak tweak dylibs live outside that directory and
// are excluded; enumerating every runtime class (tens of thousands) has been observed
// to crash the target process (SIGSEGV) a few seconds later on jailbroken devices.
// function run_show_classes_of_app() {
//     var mainPath = Process.mainModule.path
//     var appDir = mainPath.substring(0, mainPath.lastIndexOf("/") + 1)
//     var list = []
//     for (var className in ObjC.classes) {
//         if (!ObjC.classes.hasOwnProperty(className)) {
//             continue
//         }
//         try {
//             var handle = ObjC.classes[className].handle
//             var module = Process.findModuleByAddress(handle)
//             if (module !== null && module.path.indexOf(appDir) === 0) {
//                 list.push(className)
//             }
//         } catch (e) {
//             // Skip classes that can't be resolved to a module instead of failing the whole scan
//         }
//     }
//     send(list)
// };

function show_classes_of_app() {
    setImmediate(run_show_classes_of_app)
}

show_classes_of_app()