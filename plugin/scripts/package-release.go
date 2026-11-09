package main

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func main() {
	libraryPath := flag.String("library", "", "compiled plugin library")
	archivePath := flag.String("archive", "", "output ZIP archive")
	checksumPath := flag.String("checksum", "", "output SHA-256 checksum file")
	flag.Parse()
	if *libraryPath == "" || *archivePath == "" || *checksumPath == "" {
		fatalf("library, archive, and checksum are required")
	}
	archive, errPackage := packageLibrary(*libraryPath, *archivePath)
	if errPackage != nil {
		fatalf("package plugin library: %v", errPackage)
	}
	if errChecksum := writeChecksum(*checksumPath, *archivePath, archive); errChecksum != nil {
		fatalf("write checksum: %v", errChecksum)
	}
}

func packageLibrary(libraryPath, archivePath string) ([]byte, error) {
	library, errOpen := os.Open(libraryPath)
	if errOpen != nil {
		return nil, errOpen
	}
	defer library.Close()
	info, errStat := library.Stat()
	if errStat != nil {
		return nil, errStat
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("plugin library is not a regular file")
	}
	if errMkdir := os.MkdirAll(filepath.Dir(archivePath), 0o755); errMkdir != nil {
		return nil, errMkdir
	}
	archive, errCreate := os.Create(archivePath)
	if errCreate != nil {
		return nil, errCreate
	}
	writer := zip.NewWriter(archive)
	header, errHeader := zip.FileInfoHeader(info)
	if errHeader != nil {
		_ = archive.Close()
		return nil, errHeader
	}
	header.Name = filepath.Base(libraryPath)
	header.Method = zip.Deflate
	header.SetMode(0o755)
	entry, errEntry := writer.CreateHeader(header)
	if errEntry != nil {
		_ = writer.Close()
		_ = archive.Close()
		return nil, errEntry
	}
	if _, errCopy := io.Copy(entry, library); errCopy != nil {
		_ = writer.Close()
		_ = archive.Close()
		return nil, errCopy
	}
	if errClose := writer.Close(); errClose != nil {
		_ = archive.Close()
		return nil, errClose
	}
	if errClose := archive.Close(); errClose != nil {
		return nil, errClose
	}
	return os.ReadFile(archivePath)
}

func writeChecksum(checksumPath, archivePath string, data []byte) error {
	checksum := sha256.Sum256(data)
	line := fmt.Sprintf("%s  %s\n", hex.EncodeToString(checksum[:]), filepath.Base(archivePath))
	if errMkdir := os.MkdirAll(filepath.Dir(checksumPath), 0o755); errMkdir != nil {
		return errMkdir
	}
	return os.WriteFile(checksumPath, []byte(line), 0o644)
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
