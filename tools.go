package main

import (
	"bytes"
	"crypto/md5"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

type WLockFile struct {
	f  *os.File
	mu sync.RWMutex
}

// Open 打开文件
func (l *WLockFile) Open(filename string) error {
	fi, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return err
	}
	l.f = fi
	return nil
}

// Write 写入数据
func (l *WLockFile) Write(msg []byte) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	_, err := l.f.Write(msg)
	return err
}

// Close 关闭文件
func (l *WLockFile) Close() error {
	return l.f.Close()
}

var logFiles sync.Map

// GetFileContent 获取文件内容
func GetFileContent(filename string) ([]byte, error) {
	f, e := os.Open(filename)
	if e != nil {
		return nil, e
	}
	return ioutil.ReadAll(f)
}

// CheckFileExists 检查文件是否存在
func CheckFileExists(filename string) bool {
	_, err := os.Stat(filename)
	if err == nil {
		return true
	}
	return false
}

// WriteLog 写日志
// folderPrefix 日志保存目录前缀
// filename 存储的文件名
func WriteLog(folderPrefix, filename string, msg ...string) {
	if folderPrefix == "" {
		return
	}
	sep := string(os.PathSeparator)
	folderPrefix = strings.TrimRight(folderPrefix, sep)
	data := strings.Join(msg, " ")
	nowDate := time.Now().Format("20060102")
	data = GetCurrentDateTime() + " " + data + "\n"
	finalFilename := fmt.Sprintf("%s%s%s%s%s.log", folderPrefix, sep, nowDate, sep, filename)
	dirname := filepath.Dir(finalFilename)
	if !CheckFileExists(dirname) {
		err := os.MkdirAll(dirname, 0777)
		if err != nil {
			log.Println("创建目录", dirname, "失败", err)
			return
		}
	}
	//if logFiles == nil {
	//	logFiles = make(map[string]*WLockFile)
	//}
	// filename做哈希
	key := fmt.Sprintf("%x", md5.Sum([]byte(finalFilename)))
	var err error

	if fi, ok := logFiles.Load(key); ok {
		f, ok := fi.(*WLockFile)
		if !ok {
			log.Println("从map读取数据断言*WLockFile失败")
			return
		}
		// 之前已经存在，直接写入即可
		err = f.Write([]byte(data))
		if err != nil {
			log.Println("写日志失败", err)
			return
		}
	} else {
		// 不存在，实例化一个新的
		l := &WLockFile{}
		err = l.Open(finalFilename)
		if err != nil {
			log.Println("写日志失败", err)
			return
		}
		err = l.Write([]byte(data))
		if err != nil {
			log.Println("写日志失败", err)
			l.Close()
			return
		}
		logFiles.Store(key, l)
	}

}

// GetCurrentDateTime 获取当前时间
func GetCurrentDateTime() string {
	return time.Now().Format("2006-01-02 15:04:05")
}

// ExecCommand 执行shell名，阻塞，需要输出返回值
func ExecCommand(cmdStr string) (output, errStr string, err error) {
	var cmd *exec.Cmd
	var stdout, stderr bytes.Buffer
	defer func() {
		if err != nil {
			writeLog("main", "执行命令失败", cmdStr, err.Error(), stderr.String(), stdout.String())
		} else {
			writeLog("main", fmt.Sprintf("执行命令成功:%s\nout:\n%s\nerr:%s\n\n", cmdStr, stdout.String(), stderr.String()))
		}
	}()
	if runtime.GOOS == "windows" {
		cmd = exec.Command("powershell")
	} else {
		cmd = exec.Command("/bin/bash")
	}
	stdin := bytes.NewBufferString(cmdStr)
	cmd.Stdin = stdin
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Start()
	if err != nil {
		return stdout.String(), stderr.String(), err
	}
	if err = cmd.Wait(); err != nil {
		return stdout.String(), stderr.String(), err
	}
	return stdout.String(), stderr.String(), err
}
