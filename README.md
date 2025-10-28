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

## Getting Started

To connect to a SPICE host, you need to implement two interfaces:

1. **`spice.Connector`** - Provides network connectivity to the SPICE server
2. **`spice.Driver`** - Handles display rendering, input events, and clipboard operations

### The Connector Interface

The `Connector` interface establishes network connections to the SPICE server:

```go
type Connector interface {
    SpiceConnect(compress bool) (net.Conn, error)
}
```

### The Driver Interface

The `Driver` interface handles display updates, user input, cursor, and clipboard:

```go
type Driver interface {
    // Display management
    DisplayInit(image.Image)
    DisplayRefresh()

    // Input and channel setup
    SetEventsTarget(*ChInputs)
    SetMainTarget(*ChMain)
    SetCursor(img image.Image, x, y uint16)

    // Clipboard operations
    ClipboardGrabbed(selection SpiceClipboardSelection, clipboardTypes []SpiceClipboardFormat)
    ClipboardFetch(selection SpiceClipboardSelection, clType SpiceClipboardFormat) ([]byte, error)
    ClipboardRelease(selection SpiceClipboardSelection)
}
```

## Connection Examples

### Basic TCP Connection

```go
package main

import (
    "fmt"
    "net"
    "github.com/Shells-com/spice"
)

// SimpleConnector implements spice.Connector for direct TCP connections
type SimpleConnector struct {
    Host string
    Port int
}

func (c *SimpleConnector) SpiceConnect(compress bool) (net.Conn, error) {
    return net.Dial("tcp", fmt.Sprintf("%s:%d", c.Host, c.Port))
}

func main() {
    // Create connector
    connector := &SimpleConnector{
        Host: "localhost",
        Port: 5900,
    }

    // Create driver (implement spice.Driver interface - see below)
    driver := &MyDriver{}

    // Connect to SPICE server
    client, err := spice.New(connector, driver, "yourpassword")
    if err != nil {
        panic(err)
    }

    // Client is now connected and ready to use
    fmt.Println("Connected to SPICE server!")

    // Access various features
    if fileTransfer := client.GetFileTransfer(); fileTransfer != nil {
        // File transfer is available
    }
}
```

### TLS Connection

```go
import (
    "crypto/tls"
    "fmt"
    "net"
)

type TLSConnector struct {
    Host string
    Port int
}

func (c *TLSConnector) SpiceConnect(compress bool) (net.Conn, error) {
    config := &tls.Config{
        // Configure TLS settings as needed
        InsecureSkipVerify: false, // Set to true only for testing
    }

    addr := fmt.Sprintf("%s:%d", c.Host, c.Port)
    return tls.Dial("tcp", addr, config)
}
```

### WebSocket Connection

```go
import (
    "net"
    "github.com/gorilla/websocket"
)

type WebSocketConnector struct {
    URL string
}

func (c *WebSocketConnector) SpiceConnect(compress bool) (net.Conn, error) {
    conn, _, err := websocket.DefaultDialer.Dial(c.URL, nil)
    if err != nil {
        return nil, err
    }

    // Wrap websocket connection to implement net.Conn
    return &wsConn{conn: conn}, nil
}

// wsConn wraps a websocket.Conn to implement net.Conn interface
type wsConn struct {
    conn *websocket.Conn
}

func (w *wsConn) Read(p []byte) (n int, err error) {
    _, data, err := w.conn.ReadMessage()
    if err != nil {
        return 0, err
    }
    return copy(p, data), nil
}

func (w *wsConn) Write(p []byte) (n int, err error) {
    err = w.conn.WriteMessage(websocket.BinaryMessage, p)
    return len(p), err
}

// Implement other net.Conn methods...
```

### Minimal Driver Implementation

For headless operation or testing, you can implement a minimal driver:

```go
package main

import (
    "image"
    "github.com/Shells-com/spice"
)

type MinimalDriver struct {
    inputs *spice.ChInputs
    main   *spice.ChMain
}

func (d *MinimalDriver) DisplayInit(img image.Image) {
    // Store or render the initial display image
}

func (d *MinimalDriver) DisplayRefresh() {
    // Refresh the display
}

func (d *MinimalDriver) SetEventsTarget(inputs *spice.ChInputs) {
    d.inputs = inputs
}

func (d *MinimalDriver) SetMainTarget(main *spice.ChMain) {
    d.main = main
}

func (d *MinimalDriver) SetCursor(img image.Image, x, y uint16) {
    // Update cursor position and image
}

func (d *MinimalDriver) ClipboardGrabbed(selection spice.SpiceClipboardSelection,
    clipboardTypes []spice.SpiceClipboardFormat) {
    // Handle clipboard grab event
}

func (d *MinimalDriver) ClipboardFetch(selection spice.SpiceClipboardSelection,
    clType spice.SpiceClipboardFormat) ([]byte, error) {
    // Return clipboard data
    return nil, nil
}

func (d *MinimalDriver) ClipboardRelease(selection spice.SpiceClipboardSelection) {
    // Handle clipboard release
}
```

For a complete GUI implementation, see the [spicefyne](../spicefyne) package which provides a full-featured driver using the Fyne UI toolkit.

## Usage Examples

### Sending Keyboard Input

Once connected, you can send keyboard events through the inputs channel:

```go
// After connection, the driver's SetEventsTarget will be called with the inputs channel
// Store it in your driver implementation, then use it to send events

// Send a key press
inputs.KeyDown(spice.KEY_A)

// Send a key release
inputs.KeyUp(spice.KEY_A)

// Type a character
inputs.KeyPress(spice.KEY_H)  // Press and release
```

### Sending Mouse Input

```go
// Move mouse to absolute position
inputs.MouseMove(100, 200)

// Mouse button press
inputs.MouseDown(spice.MOUSE_BUTTON_LEFT)

// Mouse button release
inputs.MouseUp(spice.MOUSE_BUTTON_LEFT)

// Mouse click (press and release)
inputs.MouseClick(spice.MOUSE_BUTTON_LEFT)

// Mouse wheel scroll
inputs.MouseScroll(0, -1)  // Scroll up
inputs.MouseScroll(0, 1)   // Scroll down
```

### Audio Playback

The SPICE client automatically connects to the audio playback channel if available:

```go
client, err := spice.New(connector, driver, password)
if err != nil {
    panic(err)
}

// Audio is automatically streamed to the system audio output
// The client handles all audio decoding and playback internally
```

### Audio Recording

To capture audio from the client and send it to the server:

```go
// Get the audio recording interface
recorder := client.GetAudioRecorder()
if recorder != nil {
    // Start recording with specific audio format
    err := recorder.Start(spice.AUDIO_FMT_S16, 2, 44100)
    if err != nil {
        log.Printf("Failed to start recording: %v", err)
    }

    // Send audio data
    audioData := []int16{ /* PCM audio samples */ }
    recorder.SendSamples(audioData)

    // Stop recording
    recorder.Stop()
}
```

### Clipboard Operations

Clipboard integration allows copy/paste between client and server:

```go
// In your Driver implementation:

func (d *MyDriver) ClipboardGrabbed(selection spice.SpiceClipboardSelection,
    clipboardTypes []spice.SpiceClipboardFormat) {
    // Server has grabbed the clipboard
    // You can now request clipboard data if needed
}

func (d *MyDriver) ClipboardFetch(selection spice.SpiceClipboardSelection,
    clType spice.SpiceClipboardFormat) ([]byte, error) {
    // Server is requesting clipboard data from the client
    if clType == spice.SPICE_CLIPBOARD_FORMAT_TEXT {
        return []byte("clipboard text content"), nil
    }
    return nil, fmt.Errorf("unsupported clipboard format")
}

func (d *MyDriver) ClipboardRelease(selection spice.SpiceClipboardSelection) {
    // Server has released the clipboard
}

// To grab clipboard from client side:
// Use the main channel to announce clipboard grab
main.GrabClipboard(spice.SPICE_CLIPBOARD_SELECTION_CLIPBOARD,
    []spice.SpiceClipboardFormat{spice.SPICE_CLIPBOARD_FORMAT_TEXT})
```

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

## Connection Flow

When you call `spice.New(connector, driver, password)`, the following happens:

1. **Main Channel Connection**: The client establishes a connection to the main SPICE channel using your `Connector`. This channel is responsible for:
   - Authentication using the provided password
   - Retrieving the list of available channels from the server
   - Coordinating overall session management

2. **Capability Negotiation**: The client and server exchange supported capabilities to determine which features are available (compression, audio formats, etc.)

3. **Parallel Channel Setup**: Once the main channel is established, the client connects to all available channels in parallel:
   - **Display Channel**: Receives screen updates and rendering commands
   - **Inputs Channel**: Sends keyboard and mouse events to the server
   - **Cursor Channel**: Receives cursor shape and position updates
   - **Playback Channel**: Streams audio from the server (if available)
   - **Record Channel**: Sends audio to the server (if available)
   - **WebDAV Channel**: Handles file transfer operations (if available)

4. **Driver Initialization**: As channels connect, your `Driver` methods are called:
   - `SetMainTarget()`: Provides access to the main channel
   - `SetEventsTarget()`: Provides access to the inputs channel
   - `DisplayInit()`: Called when the initial screen image is ready
   - `SetCursor()`: Called with initial cursor information

5. **Ready to Use**: Once `spice.New()` returns, all channels are connected and the client is ready for interaction.

### Connection Notes

- The `compress` parameter in `SpiceConnect()` indicates whether the display channel should use compression. The client will request this when connecting to the display channel.
- Connections may be established multiple times during the session (one per channel: main, display, inputs, cursor, audio, etc.)
- The client handles all protocol details, compression, and channel multiplexing automatically
- Your `Connector` should return a fresh `net.Conn` for each call to `SpiceConnect()`

## Advanced Topics

### Handling Connection Failures

```go
client, err := spice.New(connector, driver, password)
if err != nil {
    // Connection failed - could be:
    // - Network error (cannot reach server)
    // - Authentication error (wrong password)
    // - Protocol error (incompatible versions)
    log.Printf("Failed to connect: %v", err)
    return
}
```

### Debug Logging

Enable debug logging to see protocol details:

```go
client, err := spice.New(connector, driver, password)
if err != nil {
    panic(err)
}

// Enable debug logging
client.Debug = log.New(os.Stdout, "SPICE: ", log.LstdFlags)
```

### Checking Available Features

Not all SPICE servers support all features. Check availability before use:

```go
// Check file transfer support
if fileTransfer := client.GetFileTransfer(); fileTransfer != nil {
    // File transfer is available
} else {
    log.Println("File transfer not supported by this server")
}

// Check audio recording support
if recorder := client.GetAudioRecorder(); recorder != nil {
    // Audio recording is available
} else {
    log.Println("Audio recording not supported by this server")
}
```

## License

See the LICENSE file in the repository root.

