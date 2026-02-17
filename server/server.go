package main

import (
	"crypto/md5"
	"file-transfer/messages"
	"file-transfer/util"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"syscall"
)

func handleStorage(msgHandler *messages.MessageHandler, request *messages.StorageRequest) {
	log.Println("Attempting to store", request.FileName)

	// Use only the base name to prevent directory traversal
	safeFileName := filepath.Base(request.FileName)

	// Check if file already exists (refuse to overwrite)
	file, err := os.OpenFile(safeFileName, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0666)
	if err != nil {
		msgHandler.SendResponse(false, err.Error())
		return
	}

	// Check available disk space
	var stat syscall.Statfs_t
	if err := syscall.Statfs(".", &stat); err == nil {
		availableBytes := stat.Bavail * uint64(stat.Bsize)
		if request.Size > availableBytes {
			file.Close()
			os.Remove(safeFileName)
			msgHandler.SendResponse(false, "Not enough disk space")
			return
		}
	}

	// Send OK so client can begin sending data
	msgHandler.SendResponse(true, "Ready for data")

	md5 := md5.New()
	w := io.MultiWriter(file, md5)
	io.CopyN(w, msgHandler, int64(request.Size))
	file.Close()

	serverCheck := md5.Sum(nil)

	clientCheckMsg, _ := msgHandler.Receive()
	clientCheck := clientCheckMsg.GetChecksum().Checksum

	if util.VerifyChecksum(serverCheck, clientCheck) {
		msgHandler.SendResponse(true, "Storage successful. Checksum verified.")
		log.Println("Successfully stored file.")
	} else {
		msgHandler.SendResponse(false, "Storage failed. Checksum mismatch.")
		os.Remove(safeFileName)
		log.Println("FAILED to store file. Invalid checksum.")
	}
}

func handleRetrieval(msgHandler *messages.MessageHandler, request *messages.RetrievalRequest) {
	log.Println("Attempting to retrieve", request.FileName)

	// Use only the base name to prevent directory traversal
	safeFileName := filepath.Base(request.FileName)

	// Get file size and make sure it exists
	info, err := os.Stat(safeFileName)
	if err != nil {
		log.Println("File not found:", err)
		msgHandler.SendRetrievalResponse(false, "File not found: "+err.Error(), 0, nil)
		return
	}

	// Pre-compute checksum so we can send it with the response
	file, err := os.Open(safeFileName)
	if err != nil {
		log.Println("Failed to open file:", err)
		msgHandler.SendRetrievalResponse(false, "Failed to open file: "+err.Error(), 0, nil)
		return
	}
	hash := md5.New()
	io.Copy(hash, file)
	checksum := hash.Sum(nil)
	file.Close()

	// Send response with size AND checksum
	msgHandler.SendRetrievalResponse(true, "Ready to send", uint64(info.Size()), checksum)

	// Stream the file data
	file, _ = os.Open(safeFileName)
	io.CopyN(msgHandler, file, info.Size())
	file.Close()

	log.Println("File sent:", safeFileName)
}

func handleClient(msgHandler *messages.MessageHandler) {
	defer msgHandler.Close()

	wrapper, err := msgHandler.Receive()
	if err != nil {
		log.Println(err)
		return
	}

	switch msg := wrapper.Msg.(type) {
	case *messages.Wrapper_StorageReq:
		handleStorage(msgHandler, msg.StorageReq)
	case *messages.Wrapper_RetrievalReq:
		handleRetrieval(msgHandler, msg.RetrievalReq)
	case nil:
		log.Println("Received an empty message, terminating client")
	default:
		log.Printf("Unexpected message type: %T", msg)
	}
	// Disconnect the client after the operation
	log.Println("Disconnecting client")
}

func main() {
	if len(os.Args) < 2 {
		fmt.Printf("Not enough arguments. Usage: %s port [download-dir]\n", os.Args[0])
		os.Exit(1)
	}

	port := os.Args[1]
	listener, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Fatalln(err.Error())
		os.Exit(1)
	}
	defer listener.Close()

	dir := "."
	if len(os.Args) >= 3 {
		dir = os.Args[2]
	}
	// Ensure storage directory exists
	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Fatalln("Failed to create storage directory:", err)
	}
	if err := os.Chdir(dir); err != nil {
		log.Fatalln(err)
	}

	fmt.Println("Listening on port:", port)
	fmt.Println("Download directory:", dir)
	for {
		if conn, err := listener.Accept(); err == nil {
			log.Println("Accepted connection", conn.RemoteAddr())
			handler := messages.NewMessageHandler(conn)
			go handleClient(handler)
		}
	}
}
