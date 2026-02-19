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
	"strings"
)

func put(msgHandler *messages.MessageHandler, fileName string) int {
	fmt.Println("PUT", fileName)

	// Get file size and make sure it exists
	info, err := os.Stat(fileName)
	if err != nil {
		log.Fatalln(err)
	}

	// Send only the base file name to the server (no directories)
	baseName := filepath.Base(fileName)
	msgHandler.SendStorageRequest(baseName, uint64(info.Size()))
	if ok, _ := msgHandler.ReceiveResponse(); !ok {
		return 1
	}

	file, err := os.Open(fileName)
	if err != nil {
		log.Fatalln(err)
	}
	md5 := md5.New()
	w := io.MultiWriter(msgHandler, md5)
	io.CopyN(w, file, info.Size())
	file.Close()

	checksum := md5.Sum(nil)
	msgHandler.SendChecksumVerification(checksum)
	if ok, _ := msgHandler.ReceiveResponse(); !ok {
		return 1
	}

	fmt.Println("Storage complete!")
	return 0
}

func get(msgHandler *messages.MessageHandler, fileName string, destDir string) int {
	fmt.Println("GET", fileName)

	// Build the destination file path
	destPath := filepath.Join(destDir, filepath.Base(fileName))

	msgHandler.SendRetrievalRequest(fileName)
	ok, _, size := msgHandler.ReceiveRetrievalResponse()
	if !ok {
		return 1
	}

	file, err := os.OpenFile(destPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0666)
	if err != nil {
		log.Println(err)
		return 1
	}

	// Receive the file data stream and compute checksum as we go
	hash := md5.New()
	w := io.MultiWriter(file, hash)
	io.CopyN(w, msgHandler, int64(size))
	file.Close()

	// Receive checksum from server and verify
	clientChecksum := hash.Sum(nil)
	checkMsg, _ := msgHandler.Receive()
	serverChecksum := checkMsg.GetChecksum().Checksum
	if util.VerifyChecksum(serverChecksum, clientChecksum) {
		fmt.Println("Successfully retrieved file.")
	} else {
		fmt.Println("FAILED to retrieve file. Invalid checksum.")
		os.Remove(destPath)
		return 1
	}

	return 0
}

func main() {
	if len(os.Args) < 4 {
		fmt.Printf("Not enough arguments. Usage: %s server:port put|get file-name [download-dir]\n", os.Args[0])
		os.Exit(1)
	}

	host := os.Args[1]
	conn, err := net.Dial("tcp", host)
	if err != nil {
		log.Fatalln(err.Error())
		return
	}
	msgHandler := messages.NewMessageHandler(conn)
	defer conn.Close()

	action := strings.ToLower(os.Args[2])
	if action != "put" && action != "get" {
		log.Fatalln("Invalid action", action)
	}

	fileName := os.Args[3]

	dir := "."
	if len(os.Args) >= 5 {
		dir = os.Args[4]
	}
	openDir, err := os.Open(dir)
	if err != nil {
		log.Fatalln(err)
	}
	openDir.Close()

	if action == "put" {
		os.Exit(put(msgHandler, fileName))
	} else if action == "get" {
		os.Exit(get(msgHandler, fileName, dir))
	}
}
