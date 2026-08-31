import ObjC from "frida-objc-bridge";

function protected_resources_permissions() {
    // console.log("[F] Started: Protected Resources Permissions Finder");

    var dictKeys = ObjC.classes.NSBundle.mainBundle().infoDictionary().allKeys();
    var permissionListArray = ["NSBluetoothAlwaysUsageDescription",
        "NSCalendarsUsageDescription",
        "NSRemindersUsageDescription",
        "NSCameraUsageDescription",
        "NSMicrophoneUsageDescription",
        "NSContactsUsageDescription",
        "NSFaceIDUsageDescription",
        "NSDesktopFolderUsageDescription",
        "NSDocumentsFolderUsageDescription",
        "NSDownloadsFolderUsageDescription",
        "NSNetworkVolumesUsageDescription",
        "NSRemovableVolumesUsageDescription",
        "NSFileProviderDomainUsageDescription",
        "NSGKFriendListUsageDescription",
        "NSHealthClinicalHealthRecordsShareUsageDescription",
        "NSHealthShareUsageDescription",
        "NSHealthUpdateUsageDescription",
        "NSHealthRequiredReadAuthorizationTypeIdentifiers",
        "NSHomeKitUsageDescription",
        "NSLocationAlwaysAndWhenInUseUsageDescription",
        "NSLocationUsageDescription",
        "NSLocationWhenInUseUsageDescription",
        "NSLocationTemporaryUsageDescriptionDictionary",
        "NSWidgetWantsLocation",
        "NSLocationDefaultAccuracyReduced",
        "NSAppleMusicUsageDescription",
        "NSMotionUsageDescription",
        "NSFallDetectionUsageDescription",
        "NSLocalNetworkUsageDescription",
        "NSNearbyInteractionUsageDescription",
        "NFCReaderUsageDescription",
        "NSPhotoLibraryAddUsageDescription",
        "NSPhotoLibraryUsageDescription",
        "NSUpdateSecurityPolicy",
        "NSUserTrackingUsageDescription",
        "NSAppleEventsUsageDescription",
        "NSSensorKitUsageDescription",
        "NSSensorKitUsageDetail",
        "NSSensorKitPrivacyPolicyURL",
        "NSSiriUsageDescription",
        "NSSpeechRecognitionUsageDescription",
        "NSVideoSubscriberAccountUsageDescription",
        "NSIdentityUsageDescription"
    ];


    var foundPermissionsWithInfo = {};

    // console.log("Number of protected resources/permissions to check: " + permissionListArray.length);
    // console.log("Found keys: " + dictKeys.toString());
    // console.log("--------------------------------------------------");

    for (var i = 0; i < dictKeys.count(); i++) {
        var key = dictKeys.objectAtIndex_(i).toString();

        if (permissionListArray.indexOf(key) !== -1) {
            // console.log("Resource : " + key);
            // console.log("Value    : " + ObjC.classes.NSBundle.mainBundle().infoDictionary().objectForKey_(key).toString());
            // console.log("");

            // Store found permission and its info
            foundPermissionsWithInfo[key] = ObjC.classes.NSBundle.mainBundle().infoDictionary().objectForKey_(key).toString();
        }
    }

    // Maybe just send the entire dict object and parse it in Go?
    send(foundPermissionsWithInfo);
}

function run_permissions() {
    protected_resources_permissions();
}

setImmediate(run_permissions);
