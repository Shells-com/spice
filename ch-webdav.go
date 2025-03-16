package spice

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// FileTransferProgress represents the progress of a file transfer
type FileTransferProgress struct {
	FileName   string  // Name of the file being transferred
	TotalSize  int64   // Total file size in bytes
	BytesSent  int64   // Bytes sent so far
	Percentage float64 // Progress percentage (0-100)
	Status     uint32  // Current status (one of VD_AGENT_FILE_XFER_STATUS_*)
	Error      error   // Error if any
}

// FileTransferCallback is called when file transfer status changes
type FileTransferCallback func(progress FileTransferProgress)

// ActiveTransfer represents an active file transfer
type ActiveTransfer struct {
	ID           uint32               // Unique transfer ID
	File         *os.File             // File handle
	FileName     string               // File name (used in progress reporting)
	OriginalPath string               // Original path on the host
	TotalSize    int64                // Total file size
	BytesSent    int64                // Bytes sent so far
	Callback     FileTransferCallback // Progress callback
}

// SpiceWebdav handles the WebDAV channel communication for file transfers
type SpiceWebdav struct {
	cl            *Client
	conn          *SpiceConn
	transfers     map[uint32]*ActiveTransfer // Active transfers
	transfersLock sync.Mutex                 // Lock for the transfers map
	nextID        uint32                     // Next transfer ID
	idLock        sync.Mutex                 // Lock for the nextID
}

// setupWebdav creates and initializes the WebDAV channel
func (cl *Client) setupWebdav(id uint8) (*SpiceWebdav, error) {
	conn, err := cl.conn(ChannelWebdav, id, nil)
	if err != nil {
		return nil, err
	}
	m := &SpiceWebdav{
		cl:        cl,
		conn:      conn,
		transfers: make(map[uint32]*ActiveTransfer),
		nextID:    1,
	}
	conn.hndlr = m.handle

	go m.conn.ReadLoop()

	return m, nil
}

// getNextID returns the next available transfer ID
func (d *SpiceWebdav) getNextID() uint32 {
	d.idLock.Lock()
	defer d.idLock.Unlock()
	id := d.nextID
	d.nextID++
	return id
}

// handle processes incoming WebDAV channel messages
func (d *SpiceWebdav) handle(typ uint16, data []byte) {
	switch typ {
	case SPICE_WEBDAV_MSG_FILE_XFER_STATUS:
		d.handleFileXferStatus(data)
	case SPICE_WEBDAV_MSG_FILE_XFER_DATA:
		d.handleFileXferData(data)
	default:
		log.Printf("spice/webdav: got message type=%d", typ)
	}
}

// handleFileXferStatus processes a file transfer status message
func (d *SpiceWebdav) handleFileXferStatus(data []byte) {
	if len(data) < 8 {
		log.Printf("spice/webdav: invalid file transfer status message")
		return
	}

	id := binary.LittleEndian.Uint32(data[0:4])
	status := binary.LittleEndian.Uint32(data[4:8])

	d.transfersLock.Lock()
	transfer, ok := d.transfers[id]
	d.transfersLock.Unlock()

	if !ok {
		log.Printf("spice/webdav: received status for unknown transfer ID: %d", id)
		return
	}

	switch status {
	case VD_AGENT_FILE_XFER_STATUS_CAN_SEND_DATA:
		// Guest is ready to receive data, start sending
		d.sendNextChunk(transfer)
	case VD_AGENT_FILE_XFER_STATUS_SUCCESS:
		// Transfer completed successfully
		if transfer.Callback != nil {
			transfer.Callback(FileTransferProgress{
				FileName:   transfer.FileName,
				TotalSize:  transfer.TotalSize,
				BytesSent:  transfer.TotalSize,
				Percentage: 100.0,
				Status:     status,
			})
		}
		d.cleanupTransfer(id)
	case VD_AGENT_FILE_XFER_STATUS_CANCELLED, VD_AGENT_FILE_XFER_STATUS_ERROR,
		VD_AGENT_FILE_XFER_STATUS_NOT_ENOUGH_SPACE, VD_AGENT_FILE_XFER_STATUS_SESSION_LOCKED,
		VD_AGENT_FILE_XFER_STATUS_DISABLED:
		// Transfer failed
		var err error
		switch status {
		case VD_AGENT_FILE_XFER_STATUS_CANCELLED:
			err = fmt.Errorf("transfer cancelled by guest")
		case VD_AGENT_FILE_XFER_STATUS_ERROR:
			err = fmt.Errorf("transfer failed with error")
		case VD_AGENT_FILE_XFER_STATUS_NOT_ENOUGH_SPACE:
			err = fmt.Errorf("not enough space on guest")
		case VD_AGENT_FILE_XFER_STATUS_SESSION_LOCKED:
			err = fmt.Errorf("guest session is locked")
		case VD_AGENT_FILE_XFER_STATUS_DISABLED:
			err = fmt.Errorf("file transfers are disabled on guest")
		}

		if transfer.Callback != nil {
			transfer.Callback(FileTransferProgress{
				FileName:   transfer.FileName,
				TotalSize:  transfer.TotalSize,
				BytesSent:  transfer.BytesSent,
				Percentage: float64(transfer.BytesSent) * 100.0 / float64(transfer.TotalSize),
				Status:     status,
				Error:      err,
			})
		}
		d.cleanupTransfer(id)
	default:
		log.Printf("spice/webdav: unknown file transfer status: %d", status)
	}
}

// handleFileXferData processes a file transfer data message (for downloads from guest)
func (d *SpiceWebdav) handleFileXferData(data []byte) {
	// Not implemented yet - for downloading files from guest
	log.Printf("spice/webdav: file download not yet implemented")
}

// cleanupTransfer removes a transfer from the active transfers map and closes the file
func (d *SpiceWebdav) cleanupTransfer(id uint32) {
	d.transfersLock.Lock()
	defer d.transfersLock.Unlock()

	transfer, ok := d.transfers[id]
	if !ok {
		return
	}

	if transfer.File != nil {
		transfer.File.Close()
	}

	delete(d.transfers, id)
}

// sendNextChunk sends the next chunk of data for a transfer
func (d *SpiceWebdav) sendNextChunk(transfer *ActiveTransfer) {
	// Read the next chunk of data from the file
	const chunkSize = 16 * 1024 // 16KB chunks
	buf := make([]byte, chunkSize)
	n, err := transfer.File.Read(buf)
	if err != nil && err != io.EOF {
		log.Printf("spice/webdav: error reading file: %v", err)
		d.sendFileXferStatus(transfer.ID, VD_AGENT_FILE_XFER_STATUS_ERROR)
		d.cleanupTransfer(transfer.ID)
		return
	}

	if n > 0 {
		// Send the data
		d.sendFileXferData(transfer.ID, buf[:n])
		transfer.BytesSent += int64(n)

		// Update progress
		if transfer.Callback != nil {
			transfer.Callback(FileTransferProgress{
				FileName:   transfer.FileName,
				TotalSize:  transfer.TotalSize,
				BytesSent:  transfer.BytesSent,
				Percentage: float64(transfer.BytesSent) * 100.0 / float64(transfer.TotalSize),
				Status:     VD_AGENT_FILE_XFER_STATUS_CAN_SEND_DATA,
			})
		}
	}

	// Check if we've reached the end of the file
	if err == io.EOF || n == 0 {
		// Send a zero-length data message to indicate end of transfer
		d.sendFileXferData(transfer.ID, []byte{})
	}
}

// sendFileXferStart sends a file transfer start message
func (d *SpiceWebdav) sendFileXferStart(id uint32, fileName string, fileSize int64) error {
	// Build the file info data in the format expected by the agent
	// The format is a simple key-value format similar to INI files
	keyFile := fmt.Sprintf("[vdagent-file-xfer]\nname=%s\nsize=%d\n", fileName, fileSize)

	// Create the message
	msgBuf := &bytes.Buffer{}
	binary.Write(msgBuf, binary.LittleEndian, id) // 4 bytes ID
	msgBuf.Write([]byte(keyFile))                 // File metadata

	return d.conn.WriteMessage(SPICE_WEBDAV_MSG_FILE_XFER_START, msgBuf.Bytes())
}

// sendFileXferData sends a chunk of file data
func (d *SpiceWebdav) sendFileXferData(id uint32, data []byte) error {
	// Create the message
	msgBuf := &bytes.Buffer{}
	binary.Write(msgBuf, binary.LittleEndian, id)                // 4 bytes ID
	binary.Write(msgBuf, binary.LittleEndian, uint32(len(data))) // 4 bytes size
	msgBuf.Write(data)                                           // File data

	return d.conn.WriteMessage(SPICE_WEBDAV_MSG_FILE_XFER_DATA, msgBuf.Bytes())
}

// sendFileXferStatus sends a file transfer status message
func (d *SpiceWebdav) sendFileXferStatus(id uint32, status uint32) error {
	// Create the message
	msgBuf := &bytes.Buffer{}
	binary.Write(msgBuf, binary.LittleEndian, id)     // 4 bytes ID
	binary.Write(msgBuf, binary.LittleEndian, status) // 4 bytes status

	return d.conn.WriteMessage(SPICE_WEBDAV_MSG_FILE_XFER_STATUS, msgBuf.Bytes())
}

// SendFile initiates a file transfer to the guest
func (d *SpiceWebdav) SendFile(filePath string, callback FileTransferCallback) (uint32, error) {
	// Open the file
	file, err := os.Open(filePath)
	if err != nil {
		return 0, fmt.Errorf("failed to open file: %w", err)
	}

	// Get file info
	fileInfo, err := file.Stat()
	if err != nil {
		file.Close()
		return 0, fmt.Errorf("failed to get file info: %w", err)
	}

	if fileInfo.IsDir() {
		file.Close()
		return 0, fmt.Errorf("cannot send directories: %s", filePath)
	}

	// Get a transfer ID
	id := d.getNextID()

	// Just use the base filename for transfer to guest
	fileName := filepath.Base(filePath)

	// Create a new transfer
	transfer := &ActiveTransfer{
		ID:           id,
		File:         file,
		FileName:     fileName,
		OriginalPath: filePath,
		TotalSize:    fileInfo.Size(),
		BytesSent:    0,
		Callback:     callback,
	}

	// Add to active transfers
	d.transfersLock.Lock()
	d.transfers[id] = transfer
	d.transfersLock.Unlock()

	// Send the start message
	err = d.sendFileXferStart(id, fileName, fileInfo.Size())
	if err != nil {
		d.cleanupTransfer(id)
		return 0, fmt.Errorf("failed to send file transfer start: %w", err)
	}

	return id, nil
}

// CancelTransfer cancels an active file transfer
func (d *SpiceWebdav) CancelTransfer(id uint32) error {
	d.transfersLock.Lock()
	_, ok := d.transfers[id]
	d.transfersLock.Unlock()

	if !ok {
		return fmt.Errorf("transfer ID not found: %d", id)
	}

	// Send a cancel status
	err := d.sendFileXferStatus(id, VD_AGENT_FILE_XFER_STATUS_CANCELLED)
	if err != nil {
		return fmt.Errorf("failed to send cancel status: %w", err)
	}

	// Clean up the transfer
	d.cleanupTransfer(id)
	return nil
}

// SendFiles sends multiple files to the guest
func (d *SpiceWebdav) SendFiles(filePaths []string, callback FileTransferCallback) ([]uint32, error) {
	ids := make([]uint32, 0, len(filePaths))
	errors := make([]error, 0)

	for _, path := range filePaths {
		id, err := d.SendFile(path, callback)
		if err != nil {
			errors = append(errors, fmt.Errorf("failed to send %s: %w", path, err))
			continue
		}
		ids = append(ids, id)
	}

	if len(errors) > 0 {
		// Combine all errors into one
		errMsg := strings.Builder{}
		errMsg.WriteString("failed to send some files: ")
		for i, err := range errors {
			if i > 0 {
				errMsg.WriteString("; ")
			}
			errMsg.WriteString(err.Error())
		}
		return ids, fmt.Errorf(errMsg.String())
	}

	return ids, nil
}
