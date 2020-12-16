package raft

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/epaxos/src/dlog"
)

const (
	FILE_PREFIX = "raft_message_"
)

type PCTFile struct {
	messagePool    map[int64]*MessageWrapper
	dispatchChan   chan *MessageWrapper
	messageCounter int64
	lock           *sync.Mutex
	ackChan        chan int64
	fileInterface  *FileInterface
}

func NewPCTFileController(c *Config) *PCTFile {

	f := NewFileInterface(c.PCTWorkingDir)

	return &PCTFile{
		messagePool:    make(map[int64]*MessageWrapper),
		dispatchChan:   make(chan *MessageWrapper, 100),
		messageCounter: 0,
		lock:           new(sync.Mutex),
		ackChan:        f.Chan(),
		fileInterface:  f,
	}
}

func (pctFile *PCTFile) NotifyMessage(message *MessageWrapper) {
	pctFile.lock.Lock()
	id := pctFile.messageCounter
	pctFile.messagePool[id] = message
	message.ID = int(id)
	pctFile.messageCounter = pctFile.messageCounter + 1
	pctFile.lock.Unlock()

	b, _ := json.Marshal(message)
	log.Printf("Received: %s\n", b)

	pctFile.fileInterface.WriteMessage(message, id)
}

func (p *PCTFile) ackmonitor() {
	for {
		select {
		case id := <-p.ackChan:
			var m *MessageWrapper
			send := false
			p.lock.Lock()
			msg, ok := p.messagePool[id]
			if ok {
				m = msg
				send = true
				delete(p.messagePool, id)
			}
			p.lock.Unlock()

			if send {
				p.dispatchChan <- m
				if b, err := json.Marshal(m); err == nil {
					log.Printf("Sending: %s\n", b)
				}
			}
		}
	}
}

func (p *PCTFile) ReceiveChan() chan *MessageWrapper {
	return p.dispatchChan
}

func (pctFile *PCTFile) Run() {
	go pctFile.ackmonitor()
	pctFile.fileInterface.Run()
}

type FileInterface struct {
	workingDir  string
	newDir      string
	sendDir     string
	ackDir      string
	messageChan chan int64
	stop        bool
}

func NewFileInterface(workingDir string) *FileInterface {

	newdir := filepath.Join(workingDir, "new")
	senddir := filepath.Join(workingDir, "send")
	ackdir := filepath.Join(workingDir, "ack")

	_ = os.MkdirAll(newdir, os.ModePerm)
	_ = os.MkdirAll(senddir, os.ModePerm)
	_ = os.MkdirAll(ackdir, os.ModePerm)

	return &FileInterface{
		workingDir,
		newdir,
		senddir,
		ackdir,
		make(chan int64, 100),
		false,
	}
}

func (f *FileInterface) Run() {
	f.monitoracks()
	// go f.tempdispatcher()
}

func (f *FileInterface) Chan() chan int64 {
	return f.messageChan
}

func (f *FileInterface) tempdispatcher() {
	// Need to send files from send folder to ack folder blindly
	for !f.stop {
		files, err := ioutil.ReadDir(f.sendDir)
		if err == nil {
			for _, file := range files {
				if strings.HasPrefix(file.Name(), FILE_PREFIX) {
					oldname := filepath.Join(f.sendDir, file.Name())
					newname := filepath.Join(f.ackDir, file.Name())
					os.Rename(oldname, newname)
				}
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func (f *FileInterface) monitoracks() {
	var err error = nil
	var files []os.FileInfo

	for !f.stop && err == nil {
		files, err = ioutil.ReadDir(f.ackDir)
		if err == nil {
			for _, file := range files {
				if strings.HasPrefix(file.Name(), FILE_PREFIX) {
					go f.dispatchMessage(file.Name())
				}
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	close(f.messageChan)
}

func (f *FileInterface) dispatchMessage(filename string) {

	path := filepath.Join(f.ackDir, filename)
	execute := false
	var messageid int64 = -1

	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	for {
		line, err := reader.ReadString('\n')
		if err == io.EOF {
			break
		}
		if err != nil {
			return
		}
		if strings.HasPrefix(line, "eventId=") {
			t := strings.Split(strings.TrimSpace(line), "=")
			messageid, err = strconv.ParseInt(t[1], 10, 64)
		}
		if strings.HasPrefix(line, "execute=") {
			t := strings.Split(strings.TrimSpace(line), "=")
			if t[1] == "true" {
				execute = true
			}
		}
	}
	_ = os.Remove(path)
	if err == nil && messageid >= 0 && execute {
		log.Printf("Got acked file %s with message id %d", file.Name(), messageid)
		f.messageChan <- messageid
	}
}

func (f *FileInterface) WriteMessage(m *MessageWrapper, id int64) {
	from := strconv.Itoa(m.From)
	to := strconv.Itoa(m.To)
	idS := strconv.FormatInt(id, 10)

	filename := FILE_PREFIX + from + "_" + to + "_" + idS

	content := "eventId=" + idS + "\n"
	content += "sender=" + from + "\n"
	content += "recv=" + to + "\n"
	content += "msgtype=" + m.M.Type + "\n"
	content += "msg=" + fmt.Sprintf("%#v", m.M) + "\n"

	go f.createAndCommitFile(filename, content)
}

func (f *FileInterface) createAndCommitFile(filename string, content string) {
	err := f.createFile(filename, content)
	if err != nil {
		return
	}
	f.commitFile(filename)
}

func (f *FileInterface) commitFile(filename string) error {
	oldpath := filepath.Join(f.newDir, filename)
	newpath := filepath.Join(f.sendDir, filename)
	return os.Rename(oldpath, newpath)
}

func (f *FileInterface) createFile(filename string, content string) error {
	dlog.Printf("Creating file %s with content %s\n", filename, content)

	file, err := os.Create(filepath.Join(f.newDir, filename))
	if err != nil {
		return err
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	writer.WriteString(content)
	return writer.Flush()
}
