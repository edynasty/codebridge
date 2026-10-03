//go:build darwin && cgo

#import <Foundation/Foundation.h>
#import <ScreenCaptureKit/ScreenCaptureKit.h>
#import <ApplicationServices/ApplicationServices.h>
#import <Carbon/Carbon.h>
#include <dirent.h>
#include <errno.h>
#include <string.h>
#include <unistd.h>
#include "permissions_darwin.h"

static NSDictionary *cb_error(NSError *error) {
    return @{ @"domain": error.domain, @"code": @(error.code),
              @"detail": error.localizedDescription };
}

static NSDictionary *cb_screen(void) {
    BOOL preflight = CGPreflightScreenCaptureAccess();
    dispatch_semaphore_t ready = dispatch_semaphore_create(0);
    __block SCShareableContent *content = nil;
    __block NSError *contentError = nil;
    [SCShareableContent getShareableContentExcludingDesktopWindows:NO onScreenWindowsOnly:YES
        completionHandler:^(SCShareableContent *value, NSError *error) {
            content = value;
            contentError = error;
            dispatch_semaphore_signal(ready);
        }];
    if (dispatch_semaphore_wait(ready, dispatch_time(DISPATCH_TIME_NOW, 10 * NSEC_PER_SEC))) {
        return @{ @"preflight_granted": @(preflight), @"shareable_content_call": @"timeout",
                  @"capture_call": @"skipped", @"pixels_persisted": @NO };
    }
    NSMutableDictionary *report = [@{ @"preflight_granted": @(preflight),
        @"shareable_content_call": contentError ? @"error" : @"ok",
        @"capture_call": @"skipped", @"pixels_persisted": @NO } mutableCopy];
    if (contentError) {
        report[@"shareable_content_error"] = cb_error(contentError);
        // Only the documented user-declined SCK error is classified as denial.
        if ([contentError.domain isEqualToString:SCStreamErrorDomain] && contentError.code == -3801)
            report[@"shareable_content_call"] = @"denied";
        return report;
    }
    report[@"display_count"] = @(content.displays.count);
    SCDisplay *display = content.displays.firstObject;
    if (!display) {
        report[@"capture_call"] = @"no_display";
        return report;
    }
    if (@available(macOS 14.0, *)) {
        SCContentFilter *filter = [[SCContentFilter alloc] initWithDisplay:display excludingWindows:@[]];
        SCStreamConfiguration *config = [[SCStreamConfiguration alloc] init];
        config.width = display.width;
        config.height = display.height;
        config.showsCursor = NO;
        dispatch_semaphore_t captured = dispatch_semaphore_create(0);
        __block NSDictionary *capture = nil;
        [SCScreenshotManager captureImageWithFilter:filter configuration:config
            completionHandler:^(CGImageRef image, NSError *error) {
                if (error) {
                    capture = @{ @"capture_call": @"error", @"capture_error": cb_error(error) };
                } else if (image) {
                    capture = @{ @"capture_call": @"ok", @"captured_width": @(CGImageGetWidth(image)),
                                 @"captured_height": @(CGImageGetHeight(image)) };
                } else {
                    capture = @{ @"capture_call": @"error", @"capture_error": @"no image returned" };
                }
                dispatch_semaphore_signal(captured);
            }];
        if (dispatch_semaphore_wait(captured, dispatch_time(DISPATCH_TIME_NOW, 10 * NSEC_PER_SEC))) {
            report[@"capture_call"] = @"timeout";
        } else {
            [report addEntriesFromDictionary:capture];
        }
    } else {
        report[@"capture_call"] = @"unsupported_os_version";
    }
    return report;
}

static NSDictionary *cb_accessibility(void) {
    AXUIElementRef system = AXUIElementCreateSystemWide();
    CFTypeRef focused = NULL;
    AXError status = AXUIElementCopyAttributeValue(system, kAXFocusedApplicationAttribute, &focused);
    if (focused) CFRelease(focused);
    CFRelease(system);
    return @{ @"trusted": @((bool)AXIsProcessTrusted()), @"api_error_code": @(status),
              @"focused_application_call": status == kAXErrorSuccess ? @"ok" :
                  (status == kAXErrorAPIDisabled ? @"denied" : @"error") };
}

typedef struct { unsigned events; unsigned disabled; } cb_tap_counts;

static CGEventRef cb_listen(CGEventTapProxy proxy, CGEventType type, CGEventRef event, void *context) {
    cb_tap_counts *counts = context;
    if (type == kCGEventTapDisabledByTimeout || type == kCGEventTapDisabledByUserInput)
        counts->disabled++;
    else
        counts->events++;
    return event; // listen-only; event content is never inspected or persisted
}

static NSDictionary *cb_input(void) {
    BOOL preflight = CGPreflightListenEventAccess();
    cb_tap_counts counts = {0};
    CGEventMask mask = CGEventMaskBit(kCGEventKeyDown) | CGEventMaskBit(kCGEventKeyUp)
        | CGEventMaskBit(kCGEventLeftMouseDown) | CGEventMaskBit(kCGEventRightMouseDown)
        | CGEventMaskBit(kCGEventScrollWheel) | CGEventMaskBit(kCGEventFlagsChanged);
    CFMachPortRef tap = CGEventTapCreate(kCGSessionEventTap, kCGHeadInsertEventTap,
        kCGEventTapOptionListenOnly, mask, cb_listen, &counts);
    NSMutableDictionary *report = [@{ @"preflight_listen_access": @(preflight),
        @"tap_created": @((bool)(tap != NULL)), @"secure_input_enabled": @((bool)IsSecureEventInputEnabled()),
        @"injected_events": @0 } mutableCopy];
    if (!tap) {
        report[@"call"] = @"unavailable"; // TCC attribution must establish the denial cause
        report[@"detail"] = @"CGEventTapCreate returned NULL";
        return report;
    }
    CFRunLoopSourceRef source = CFMachPortCreateRunLoopSource(kCFAllocatorDefault, tap, 0);
    if (!source) {
        CFMachPortInvalidate(tap);
        CFRelease(tap);
        report[@"call"] = @"error";
        return report;
    }
    CFRunLoopRef loop = CFRunLoopGetCurrent();
    CFRunLoopAddSource(loop, source, kCFRunLoopDefaultMode);
    CGEventTapEnable(tap, true);
    CFAbsoluteTime deadline = CFAbsoluteTimeGetCurrent() + 3;
    while (CFAbsoluteTimeGetCurrent() < deadline)
        CFRunLoopRunInMode(kCFRunLoopDefaultMode, 0.1, false);
    CGEventTapEnable(tap, false);
    CFRunLoopRemoveSource(loop, source, kCFRunLoopDefaultMode);
    CFMachPortInvalidate(tap);
    CFRelease(source);
    CFRelease(tap);
    report[@"observed_events"] = @(counts.events);
    report[@"disabled_events"] = @(counts.disabled);
    report[@"call"] = counts.events && !counts.disabled && preflight ? @"ok" : @"delivery_unproven";
    return report;
}

static NSDictionary *cb_folder(NSString *path) {
    DIR *dir = opendir(path.fileSystemRepresentation);
    if (!dir) return @{ @"path": path, @"readable": @NO, @"errno": @(errno) };
    unsigned count = 0;
    struct dirent *entry;
    errno = 0;
    while ((entry = readdir(dir))) {
        if (strcmp(entry->d_name, ".") && strcmp(entry->d_name, "..")) count++;
    }
    int readError = errno;
    closedir(dir);
    return @{ @"path": path, @"readable": @((bool)(readError == 0)), @"errno": @(readError),
              @"entry_count": @(count) };
}

char *cb_daemon_permissions(int folders_only, const char *signing) {
    @autoreleasepool {
        NSMutableDictionary *report = [@{ @"pid": @(getpid()), @"parent_pid": @(getppid()),
            @"execution": @"daemon_in_process", @"started_at": @([[NSDate date] timeIntervalSince1970]),
            @"probe": folders_only ? @"daemon-files-folders" : @"daemon-permissions",
            @"signing": [NSString stringWithUTF8String:signing] } mutableCopy];
        if (folders_only) {
            NSMutableArray *roots = [NSMutableArray array];
            for (NSString *name in @[@"Desktop", @"Documents", @"Downloads"]) {
                NSString *path = [NSHomeDirectory() stringByAppendingPathComponent:name];
                dispatch_semaphore_t ready = dispatch_semaphore_create(0);
                __block NSDictionary *root = nil;
                dispatch_async(dispatch_get_global_queue(QOS_CLASS_UTILITY, 0), ^{
                    root = cb_folder(path);
                    dispatch_semaphore_signal(ready);
                });
                if (dispatch_semaphore_wait(ready, dispatch_time(DISPATCH_TIME_NOW, 5 * NSEC_PER_SEC)))
                    [roots addObject:@{ @"path": path, @"call": @"timeout", @"access": @"unresolved" }];
                else
                    [roots addObject:root];
            }
            report[@"files_folders"] = roots;
        } else {
            report[@"screen_capture"] = cb_screen();
            report[@"accessibility"] = cb_accessibility();
            report[@"input_monitor"] = cb_input();
        }
        report[@"ended_at"] = @([[NSDate date] timeIntervalSince1970]);
        NSData *data = [NSJSONSerialization dataWithJSONObject:report options:0 error:NULL];
        if (!data) return NULL;
        return strndup((const char *)data.bytes, data.length);
    }
}
