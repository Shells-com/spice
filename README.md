[![GoDoc](https://godoc.org/github.com/Shells-com/spice?status.svg)](https://godoc.org/github.com/Shells-com/spice)

# Spice

Spice protocol is a kind of remote control protocol commonly used with libvirt
and Qemu.

More information on spice: https://www.spice-space.org/

Features:

* [x] Display support (single display)
* [x] Mouse and keyboard control (client mode)
* [x] Audio playback
* [x] Clipboard
* [x] Audio recording
* [ ] USB Support
* [x] File transfer
* [ ] Video streaming

## File Transfer Example

```go
// Get the file transfer interface
fileTransfer := client.GetFileTransfer()
if fileTransfer != nil {
    // Create a progress callback
    progressCb := func(progress spice.FileTransferProgress) {
        fmt.Printf("Transfer %s: %.1f%% (%d/%d bytes)\n", 
            progress.FileName, 
            progress.Percentage, 
            progress.BytesSent, 
            progress.TotalSize)
            
        if progress.Error != nil {
            fmt.Printf("Error: %v\n", progress.Error)
        }
    }
    
    // Send a file to the guest
    transferID, err := fileTransfer.SendFile("/path/to/file.txt", progressCb)
    if err != nil {
        log.Fatalf("Failed to start file transfer: %v", err)
    }
    
    fmt.Printf("File transfer started with ID: %d\n", transferID)
    
    // To cancel a transfer (if needed):
    // fileTransfer.CancelTransfer(transferID)
    
    // Send multiple files
    ids, err := fileTransfer.SendFiles([]string{
        "/path/to/file1.txt",
        "/path/to/file2.png",
    }, progressCb)
    if err != nil {
        log.Printf("Some transfers failed: %v", err)
    }
    fmt.Printf("Started %d file transfers\n", len(ids))
}
```

