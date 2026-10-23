//go:build darwin && cgo

// Package secret stores app-owned secrets directly through the macOS Keychain API.
package secret

/*
#cgo LDFLAGS: -framework Security -framework CoreFoundation
#include <Security/Security.h>
#include <CoreFoundation/CoreFoundation.h>
#include <stdlib.h>
#include <string.h>
static CFMutableDictionaryRef query(const char *service,const char *account){
 CFMutableDictionaryRef q=CFDictionaryCreateMutable(NULL,0,&kCFTypeDictionaryKeyCallBacks,&kCFTypeDictionaryValueCallBacks);
 CFStringRef s=CFStringCreateWithCString(NULL,service,kCFStringEncodingUTF8),a=CFStringCreateWithCString(NULL,account,kCFStringEncodingUTF8);
 CFDictionarySetValue(q,kSecClass,kSecClassGenericPassword);CFDictionarySetValue(q,kSecAttrService,s);CFDictionarySetValue(q,kSecAttrAccount,a);CFRelease(s);CFRelease(a);return q;
}
static OSStatus setSecret(const char *service,const char *account,const void *value,CFIndex length){
 CFMutableDictionaryRef q=query(service,account);CFDataRef d=CFDataCreate(NULL,value,length);
 CFMutableDictionaryRef attrs=CFDictionaryCreateMutable(NULL,0,&kCFTypeDictionaryKeyCallBacks,&kCFTypeDictionaryValueCallBacks);CFDictionarySetValue(attrs,kSecValueData,d);
 OSStatus status=SecItemUpdate(q,attrs);if(status==errSecItemNotFound){CFDictionarySetValue(q,kSecValueData,d);CFDictionarySetValue(q,kSecAttrAccessible,kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly);status=SecItemAdd(q,NULL);}CFRelease(d);CFRelease(attrs);CFRelease(q);return status;
}
static OSStatus getSecret(const char *service,const char *account,void **out,long *length){
 CFMutableDictionaryRef q=query(service,account);CFDictionarySetValue(q,kSecReturnData,kCFBooleanTrue);CFDictionarySetValue(q,kSecMatchLimit,kSecMatchLimitOne);CFTypeRef result=NULL;OSStatus status=SecItemCopyMatching(q,&result);if(status==errSecSuccess){CFDataRef data=(CFDataRef)result;*length=CFDataGetLength(data);*out=malloc(*length);memcpy(*out,CFDataGetBytePtr(data),*length);CFRelease(result);}CFRelease(q);return status;
}
static OSStatus deleteSecret(const char *service,const char *account){CFMutableDictionaryRef q=query(service,account);OSStatus status=SecItemDelete(q);CFRelease(q);return status;}
*/
import "C"
import (
	"errors"
	"unsafe"
)

func Set(service, account, value string) error {
	s, a, v := C.CString(service), C.CString(account), C.CString(value)
	defer C.free(unsafe.Pointer(s))
	defer C.free(unsafe.Pointer(a))
	defer C.free(unsafe.Pointer(v))
	if status := C.setSecret(s, a, unsafe.Pointer(v), C.CFIndex(len(value))); status != 0 {
		return errors.New("macOS Keychain could not save the credential; unlock the login keychain and retry")
	}
	return nil
}
func Get(service, account string) (string, error) {
	s, a := C.CString(service), C.CString(account)
	defer C.free(unsafe.Pointer(s))
	defer C.free(unsafe.Pointer(a))
	var value unsafe.Pointer
	var size C.long
	if status := C.getSecret(s, a, &value, &size); status != 0 {
		if status == -25300 {
			return "", ErrNotFound
		}
		return "", errors.New("macOS Keychain credential is unavailable; unlock the login keychain or repair this agent connection")
	}
	defer C.free(value)
	return C.GoStringN((*C.char)(value), C.int(size)), nil
}
func Delete(service, account string) error {
	s, a := C.CString(service), C.CString(account)
	defer C.free(unsafe.Pointer(s))
	defer C.free(unsafe.Pointer(a))
	status := C.deleteSecret(s, a)
	if status != 0 && status != -25300 {
		return errors.New("macOS Keychain could not delete the credential")
	}
	return nil
}
