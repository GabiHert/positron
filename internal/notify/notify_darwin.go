package notify

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Foundation -framework UserNotifications
#include <stdlib.h>
#import <Foundation/Foundation.h>
#import <UserNotifications/UserNotifications.h>

// positronNotify posts a notification. It returns 1 without a bundle (go run, tests), where
// UNUserNotificationCenter is not available.
static int positronNotify(const char *ident, const char *title, const char *subtitle, const char *body,
		const char *thread, int sound, const char *image, const char *infoJSON) {
	@autoreleasepool {
		if ([NSBundle mainBundle].bundleIdentifier == nil) return 1;
		UNMutableNotificationContent *c = [[UNMutableNotificationContent alloc] init];
		c.title = [NSString stringWithUTF8String:title];
		c.subtitle = [NSString stringWithUTF8String:subtitle];
		c.body = [NSString stringWithUTF8String:body];
		c.threadIdentifier = [NSString stringWithUTF8String:thread];
		if (sound) c.sound = [UNNotificationSound defaultSound];
		NSData *json = [[NSString stringWithUTF8String:infoJSON] dataUsingEncoding:NSUTF8StringEncoding];
		NSDictionary *info = [NSJSONSerialization JSONObjectWithData:json options:0 error:nil];
		if ([info isKindOfClass:[NSDictionary class]]) c.userInfo = info;
		if (image[0] != 0) {
			NSURL *u = [NSURL fileURLWithPath:[NSString stringWithUTF8String:image]];
			UNNotificationAttachment *a = [UNNotificationAttachment attachmentWithIdentifier:@"picture" URL:u options:nil error:nil];
			if (a != nil) c.attachments = @[a];
		}
		UNNotificationRequest *r = [UNNotificationRequest requestWithIdentifier:[NSString stringWithUTF8String:ident] content:c trigger:nil];
		[[UNUserNotificationCenter currentNotificationCenter] addNotificationRequest:r withCompletionHandler:^(NSError *err) {
			if (err != nil) NSLog(@"positron: notification: %@", err);
		}];
		return 0;
	}
}
*/
import "C"

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"unsafe"
)

// Post shows n. Without an app bundle it does nothing and says so.
func Post(n Notification) error {
	info, _ := json.Marshal(n.UserInfo)
	if n.UserInfo == nil {
		info = []byte("{}")
	}
	image := ""
	if n.Image != "" {
		image, _ = tempCopy(n.Image) // a notification without its picture is still worth showing
	}
	cs := func(s string) *C.char { return C.CString(s) }
	args := []*C.char{cs(n.ID), cs(n.Title), cs(n.Subtitle), cs(n.Body), cs(n.Thread), cs(image), cs(string(info))}
	defer func() {
		for _, a := range args {
			C.free(unsafe.Pointer(a))
		}
	}()
	sound := C.int(0)
	if n.Sound {
		sound = 1
	}
	if C.positronNotify(args[0], args[1], args[2], args[3], args[4], sound, args[5], args[6]) != 0 {
		if image != "" {
			_ = os.Remove(image)
		}
		return errors.New("notifications need Positron.app")
	}
	return nil
}

// tempCopy copies the picture where macOS can take it.
func tempCopy(src string) (string, error) {
	in, err := os.Open(src)
	if err != nil {
		return "", err
	}
	defer in.Close()
	ext := filepath.Ext(src)
	if ext == "" {
		ext = ".jpg"
	}
	out, err := os.CreateTemp("", "positron-notify-*"+ext)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(out.Name())
		return "", err
	}
	return out.Name(), out.Close()
}
