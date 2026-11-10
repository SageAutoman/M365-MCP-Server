package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "extract-worker" {
		if os.Getenv("M365_SECRET_TEST") != "" {
			os.Exit(2)
		}
		if runExtractionWorker() != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestWindowsProtectedStorageAndProcessLock(t *testing.T) {
	dir := t.TempDir()
	if err := protectDirectory(dir); err != nil {
		t.Fatal(err)
	}
	descriptor, err := windows.GetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	dacl, _, err := descriptor.DACL()
	if err != nil || dacl == nil || dacl.AceCount != 1 {
		t.Fatalf("credential directory must have exactly one explicit grant: %s (%v)", descriptor.String(), err)
	}
	control, _, err := descriptor.Control()
	if err != nil || control&windows.SE_DACL_PROTECTED == 0 {
		t.Fatalf("credential directory inherits permissions: %s (%v)", descriptor.String(), err)
	}
	var ace *windows.ACCESS_ALLOWED_ACE
	if err := windows.GetAce(dacl, 0, &ace); err != nil {
		t.Fatal(err)
	}
	// SDDL may abbreviate a built-in account's SID. Compare the actual SID bytes.
	grantedSID := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
	if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE || !grantedSID.Equals(user.User.Sid) {
		t.Fatalf("credential directory grants access beyond the current user: %s", descriptor.String())
	}
	plain := []byte("test refresh credential")
	path := filepath.Join(dir, "cache.dpapi")
	if err := saveProtected(path, plain); err != nil {
		t.Fatal(err)
	}
	onDisk, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(onDisk, plain) {
		t.Fatal("credentials were stored in plaintext")
	}
	decoded, err := loadProtected(path)
	if err != nil || !bytes.Equal(decoded, plain) {
		t.Fatal("DPAPI round trip failed")
	}
	if err := saveProtected(path, []byte("rotated credential")); err != nil {
		t.Fatal("could not atomically replace the encrypted cache")
	}
	if updated, err := loadProtected(path); err != nil || string(updated) != "rotated credential" {
		t.Fatal("cache rotation was not persisted")
	}
	onDisk[len(onDisk)-1] ^= 1
	if _, err := unprotectData(onDisk); err == nil {
		t.Fatal("accepted tampered DPAPI data")
	}
	unlock, err := lockState(dir)
	if err != nil {
		t.Fatal(err)
	}
	if release, err := lockState(dir); err == nil {
		release()
		unlock()
		t.Fatal("two processes can own the same account")
	}
	unlock()
	unlock, err = lockState(dir)
	if err != nil {
		t.Fatal("process lock was not released")
	}
	unlock()
}

func TestWindowsIsolatedAttachmentWorker(t *testing.T) {
	t.Setenv("M365_SECRET_TEST", "must not reach parser")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	result, err := isolatedExtraction(ctx, fileAttachment{Name: "notes.txt", Content: []byte("isolated hello")})
	if err != nil || result.Content != "isolated hello" {
		t.Fatalf("Windows worker failed: %+v %v", result, err)
	}
	if _, err := isolatedExtraction(ctx, fileAttachment{Name: "invalid.pdf", Content: []byte("not a pdf")}); err == nil {
		t.Fatal("worker accepted invalid PDF")
	}
}
