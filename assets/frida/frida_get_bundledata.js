import ObjC from "frida-objc-bridge";

function toJSON(value) {
    if (value === null || value === undefined) {
        return null;
    }

    if (value.isKindOfClass_(ObjC.classes.NSString)) {
        return value.toString();
    }

    if (value.isKindOfClass_(ObjC.classes.NSNumber)) {
        const stringValue = value.toString();
        return stringValue === "0" || stringValue === "1"
            ? value.boolValue()
            : Number(stringValue);
    }

    if (value.isKindOfClass_(ObjC.classes.NSArray)) {
        const result = [];
        for (let index = 0; index < value.count(); index++) {
            result.push(toJSON(value.objectAtIndex_(index)));
        }
        return result;
    }

    if (value.isKindOfClass_(ObjC.classes.NSDictionary)) {
        const result = {};
        const keys = value.allKeys();
        for (let index = 0; index < keys.count(); index++) {
            const key = keys.objectAtIndex_(index).toString();
            result[key] = toJSON(value.objectForKey_(key));
        }
        return result;
    }

    return value.toString();
}

const bundle = ObjC.classes.NSBundle.mainBundle();
const info = bundle.infoDictionary();

send({
    displayName: toJSON(info.objectForKey_("CFBundleDisplayName")),
    bundleName: toJSON(info.objectForKey_("CFBundleName")),
    shortVersion: toJSON(info.objectForKey_("CFBundleShortVersionString")),
    buildVersion: toJSON(info.objectForKey_("CFBundleVersion")),
    executable: toJSON(info.objectForKey_("CFBundleExecutable")),
    minimumIOS: toJSON(info.objectForKey_("MinimumOSVersion")),
    supportedPlatforms: toJSON(info.objectForKey_("CFBundleSupportedPlatforms")),
    developmentRegion: toJSON(info.objectForKey_("CFBundleDevelopmentRegion")),
    deviceFamily: toJSON(info.objectForKey_("UIDeviceFamily")),
    urlTypes: toJSON(info.objectForKey_("CFBundleURLTypes")),
    querySchemes: toJSON(info.objectForKey_("LSApplicationQueriesSchemes")),
    backgroundModes: toJSON(info.objectForKey_("UIBackgroundModes")),
    ats: toJSON(info.objectForKey_("NSAppTransportSecurity")),
    encryption: toJSON(info.objectForKey_("ITSAppUsesNonExemptEncryption")),
});

