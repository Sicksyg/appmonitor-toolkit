import ObjC from "frida-objc-bridge";

// Script to open appstore link in iOS app. Semi-automate the process of purchasing and downloading apps from the appstore

function openURL(storelink) {
	const UIApplication = ObjC.classes.UIApplication.sharedApplication()
	const toOpen = ObjC.classes.NSURL.URLWithString_(storelink)
	return UIApplication.openURL_(toOpen)
};

rpc.exports = {
	openurl: function (storelink) {
		return openURL(storelink);
	},
	failPlease: function () {
		oops;
	}
};