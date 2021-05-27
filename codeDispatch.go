package main

import (
	_ "embed"
	"encoding/json"
	"encoding/xml"
	"flag"
	"fmt"
	"github.com/gookit/color"
	"os"
	"strings"
)

type ConfigFile struct {
	Programs    []ProgramItem `json:"programs"`
	IgnoreFiles []string      `json:"ignore_files"`
	DestHosts   []DestHost    `json:"dest_hosts"`
	SavePath    string        `json:"save_path"`
}
type ProgramItem struct {
	ProgramName       string   `json:"program_name"`
	ProgramPath       string   `json:"program_path"`
	DestPath          string   `json:"dest_path"`
	Scripts           []string `json:"scripts"`
	IgnoreFiles       []string `json:"ignore_files"`
	GoExecPath        string   `json:"go_exec_path"`        // go的命令路径，存在此变量证明是一个go项目，会配合-g命令进行go build
	BuildSourcePrefix string   `json:"build_source_prefix"` // 配合-g使用，最终build命令为: ProgramPath + BuildSourcePrefix + 具体项目
}

type DestHost struct {
	Username string `json:"username"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	KeyFile  string `json:"key_file"`
	Alias    string `json:"alias"`
}

type RsyncResult struct {
	DestHost
	IsSuc bool // 是否成功
}

type ProgramResult struct {
	ProgramItem
	IsSuc bool // 是否成功
}

type RuntimeArgs struct {
	JsonConfig     ConfigFile
	ConfigFileName string
	Init           bool
	Programs       string
	GoPrograms     string
	NeedHelp       bool
	Revert         bool   // 是否版本回退
	To             string // 指定发布服务器，为空表示全部发布
}

type SVNLogs struct {
	XMLName  xml.Name     `xml:"log"`
	Logentry []SVNLogItem `xml:"logentry"`
}
type SVNLogItem struct {
	XMLName  xml.Name `xml:"logentry"`
	Revision int      `xml:"revision,attr"`
	Author   string   `xml:"author"`
	Date     string   `xml:"date"`
	Msg      string   `xml:"msg"`
}

//go:embed config.json
var configJsonFileData []byte

var ra RuntimeArgs

func main() {
	// 重写usage函数
	var programs []string
	for _, configPro := range ra.JsonConfig.Programs {
		programs = append(programs, configPro.ProgramName)
	}
	var pubHosts []string
	for _, host := range ra.JsonConfig.DestHosts {
		pubHosts = append(pubHosts, fmt.Sprintf("%s[%s@%s]", host.Alias, host.Username, host.Host))
	}
	var f = func() {
		color.Comment.Printf("Usage of %s\n支持上线的项目有（需要使用-c指定配置文件才可正确展示）:\n\t|-%s\n发布的服务器包含：\n\t|-%s\n\n其他参数说明如下：\n\n", os.Args[0], strings.Join(programs, "\n\t|-"), strings.Join(pubHosts, "\n\t|-"))
		flag.PrintDefaults()
	}
	flag.Usage = f
	if ra.NeedHelp {
		flag.Usage()
		return
	}
	// 检查是否首次执行，初始化
	if ra.Init {
		firstRunEnv()
	}
	// 检查通过cli传递的项目名称是否已经在*.json文件里面配置了
	if ra.Programs == "" {
		color.Danger.Println("必须指定一个项目发布，可以使用-p参数进行指定")
		return
	}
	if ra.Programs != "all" {
		programs = strings.Split(ra.Programs, ",")
	}

	// 检查t命令是否正确
	if ra.To != "" {
		alias := make([]string, 0, len(ra.JsonConfig.DestHosts))
		var ok bool
		for _, host := range ra.JsonConfig.DestHosts {
			if host.Alias == ra.To {
				ok = true
				sep := strings.Repeat("-", 70)
				color.Info.Printf("%s\n本次仅发布服务器：%s[%s@%s]\n%s\n", sep, host.Alias, host.Username, host.Host, sep)
				break
			}
			alias = append(alias, host.Alias)
		}
		if !ok {
			color.Error.Printf("-t命令错误，输入为：%s，支持列表为：%s\n", ra.To, strings.Join(alias, ","))
			os.Exit(-1)
		}
	}
	programNum := len(programs)
	buffChan := make(chan ProgramResult, programNum)

	for _, pg := range programs {
		// 检查项目是否在json里面配置了
		config, ex := getProgramConfig(pg)
		if !ex {
			color.Danger.Println(pg, "项目尚未在json里面文件里面配置")
			buffChan <- ProgramResult{
				ProgramItem: ProgramItem{
					ProgramName: pg,
				},
				IsSuc: false,
			}
			continue
		}

		// 检查项目路径是否存在
		if !CheckFileExists(config.ProgramPath) {
			color.Danger.Printf("%s 文件路径:%s 不存在\n", pg, config.ProgramPath)
			buffChan <- ProgramResult{
				ProgramItem: ProgramItem{
					ProgramName: pg,
				},
				IsSuc: false,
			}
			continue
		}
		// 开始执行
		go startDispatch(config, buffChan)
	}
	var failPros []string
	for i := 0; i < programNum; i++ {
		result := <-buffChan
		if result.IsSuc == false {
			failPros = append(failPros, result.ProgramName)
		}
	}
	close(buffChan)
	failNum := len(failPros)
	color.Primary.Printf("\n\n上线结束，上线总项目%d个，成功%d个，失败%d个，其中失败项目为：%s\n\n", programNum, programNum-failNum, failNum, strings.Join(failPros, ","))
}

// firstRunEnv 首次执行
func firstRunEnv() {
	hasErr := false
	// 首次执行其实就是尝试连接远程服务器，如果连接成功就ok，否则报错
	for _, host := range ra.JsonConfig.DestHosts {
		color.Warn.Printf("尝试连接%s@%s\n", host.Username, host.Host)
		cmdStr := fmt.Sprintf(`ssh %s@%s -i %s -p %d`, host.Username, host.Host, host.KeyFile, host.Port)
		_, _, err := ExecCommand(cmdStr)
		if err != nil {
			color.Error.Printf("尝试连接服务器%s@%s失败，原因为：%s\n", host.Username, host.Host, err)
			hasErr = true
		} else {
			color.Success.Printf("连接%s@%s成功\n\n", host.Username, host.Host)
		}
	}
	if hasErr {
		color.Info.Println("初始化失败")
		os.Exit(-1)
	}
}

// 开始执行
func startDispatch(program ProgramItem, proBuffChan chan ProgramResult) {
	result := ProgramResult{
		ProgramItem: program,
		IsSuc:       false,
	}
	defer func() {
		proBuffChan <- result
	}()
	programName := program.ProgramName
	// 判断是更新还是回退
	var cmdStr string
	if ra.Revert {
		// 执行后退操作
		prevRevision, err := getProgramPrevRevision(program.ProgramPath)
		if err != nil {
			color.Error.Println(programName, "回滚失败", "获取上一版本失败", err)
			return
		}
		cmdStr = fmt.Sprintf("cd %s && svn up -r %d", program.ProgramPath, prevRevision.Revision)
		color.Info.Printf("%s 回滚到版本:%d，作者：%s, 日期：%s，版本消息：%s\n", programName, prevRevision.Revision, prevRevision.Author, prevRevision.Date, prevRevision.Msg)
	} else {
		// 开始执行svn更新程序
		cmdStr = fmt.Sprintf("cd %s && %s", program.ProgramPath, "svn up")
	}
	color.Comment.Println(programName, "开始", cmdStr)
	stdout, stderr, err := ExecCommand(cmdStr)
	if err != nil {
		color.Error.Println(programName, "失败", cmdStr)
		return
	}
	writeLog(programName, "执行命令成功", cmdStr, "output: ", stdout, "stderr: ", stderr)
	color.Success.Println(programName, "成功", cmdStr)
	// 检查是否有更新后脚本需要执行，比如mwx
	if len(program.Scripts) > 0 {
		for _, cmd := range program.Scripts {
			cmdStr = fmt.Sprintf("cd %s && %s", program.ProgramPath, cmd)
			color.Comment.Println(programName, "开始", cmdStr)
			stdout, stderr, err = ExecCommand(cmdStr)
			if err != nil {
				color.Error.Println(programName, "失败", cmdStr)
				return
			}
			writeLog(programName, "执行命令成功", cmdStr, "output: ", stdout, "stderr: ", stderr)
			color.Success.Println(programName, "成功", cmdStr)
		}
	}

	// 如果是go项目，那么还需要编译go文件
	if program.GoExecPath != "" && ra.GoPrograms != "" {
		goPrograms := strings.Split(ra.GoPrograms, ",")
		for _, goPro := range goPrograms {
			cmdStr = fmt.Sprintf("cd %s%s/%s/ && %s build %s.go",
				program.ProgramPath, program.BuildSourcePrefix, goPro, program.GoExecPath, goPro)
			color.Comment.Println(programName, "开始", cmdStr)
			stdout, stderr, err = ExecCommand(cmdStr)
			if err != nil {
				color.Error.Println(programName, "失败", cmdStr)
				return
			}
			writeLog(programName, "执行命令成功", cmdStr, "output: ", stdout, "stderr: ", stderr, "\n\n")
			color.Success.Println(programName, "成功", cmdStr)
		}
	}

	// 通过rsync 发布文件
	hostNum := len(ra.JsonConfig.DestHosts)
	if ra.To != "" {
		hostNum = 1
	}
	buffChan := make(chan RsyncResult, hostNum)
	for _, host := range ra.JsonConfig.DestHosts {
		if ra.To != "" && ra.To != host.Alias {
			continue
		}
		cmdStr := fmt.Sprintf(`rsync -rtzPv --progress  -e "ssh -i %s -p %d" %s %s@%s:%s`, host.KeyFile, host.Port,
			program.ProgramPath, host.Username, host.Host, program.DestPath)
		// 添加排除文件
		excludeFiles := strings.Join(append(ra.JsonConfig.IgnoreFiles, program.IgnoreFiles...), " --exclude=")
		cmdStr = fmt.Sprintf(`%s --exclude=%s`, cmdStr, excludeFiles)
		go doRsync(programName, cmdStr, host, buffChan)
	}
	sucNum := 0
	for i := 0; i < hostNum; i++ {
		result := <-buffChan
		if result.IsSuc {
			// rsync 成功
			sucNum++
			color.Success.Println(fmt.Sprintf("%s 成功 rsync到%s[%s]", programName, result.Alias, result.Host))
		} else {
			color.Error.Println(fmt.Sprintf("%s 失败 rsync到%s[%s]", programName, result.Alias, result.Host))
		}
	}
	close(buffChan)
	if ra.Revert {
		color.Info.Println("\n\n回滚结果如下：\n\n")
	}
	if sucNum == hostNum {
		result.IsSuc = true
		color.Success.Println("---------------------------------------------------------------------------------------------")
		color.Success.Println("------------------------------", programName, "全部 同步成功 --------------------------------------------")
		color.Success.Println("---------------------------------------------------------------------------------------------\n\n")
	} else if sucNum == 0 {
		color.Danger.Println("---------------------------------------------------------------------------------------------")
		color.Danger.Println("------------------------------", programName, "全部 同步失败 --------------------------------------------")
		color.Danger.Println("---------------------------------------------------------------------------------------------\n\n")
	} else {
		color.Warn.Println("---------------------------------------------------------------------------------------------")
		color.Warn.Println("------------------------------", programName, "部分 同步成功 --------------------------------------------")
		color.Warn.Println("---------------------------------------------------------------------------------------------\n\n")
	}
}

// 获取当前项目的SVN上一个(prev)版本号revision
func getProgramPrevRevision(programDir string) (SVNLogItem, error) {
	svnStr := fmt.Sprintf("svn log -l 2 --xml %s", programDir)
	output, _, err := ExecCommand(svnStr)
	if err != nil {
		return SVNLogItem{}, err
	}
	startIndex := strings.Index(output, "<?xml")
	endIndex := strings.Index(output, "</log>")
	output = output[startIndex : endIndex+6]
	var x SVNLogs
	err = xml.Unmarshal([]byte(output), &x)
	if err != nil {
		return SVNLogItem{}, err
	}
	if len(x.Logentry) != 2 {
		return SVNLogItem{}, fmt.Errorf("%s项目没有上一个版本，无法回退", programDir)
	}
	return x.Logentry[1], nil
}

// 执行rsync命令
func doRsync(programName, cmdStr string, host DestHost, buffChan chan RsyncResult) {
	result := RsyncResult{
		DestHost: host,
		IsSuc:    false,
	}
	defer func() {
		buffChan <- result
	}()
	color.Comment.Println(programName, "开始", cmdStr)
	output, stderr, err := ExecCommand(cmdStr)
	if err != nil {
		color.Error.Println(programName, "失败", cmdStr, err)
		return
	}
	writeLog(programName, "执行命令成功", cmdStr, "output: ", output, "stderr: ", stderr)
	color.Success.Println(programName, "成功", cmdStr)
	result.IsSuc = true
}

func init() {
	flag.StringVar(&ra.ConfigFileName, "c", "config.json", "配置文件地址，json格式")
	flag.StringVar(&ra.Programs, "p", "", "指定本次上线的具体项目，多个之间逗号隔开，如：api,vue-web，同步所有项目可以使用all")
	flag.StringVar(&ra.GoPrograms, "g", "", "指定本次上线的go项目里面具体程序，多个用都好隔开")
	flag.BoolVar(&ra.NeedHelp, "h", false, "使用帮助")
	flag.BoolVar(&ra.Revert, "r", false, "指定是否本次是版本回退操作")
	flag.BoolVar(&ra.Init, "i", false, "初始化，尝试连接远程服务器，只需执行一次")
	flag.StringVar(&ra.To, "t", "", "代码发布服务器，默认所有配置服务器，值为对应json配置文件里面的dest_hosts.alias")
	flag.Parse()
	var err error
	// 优先使用配置文件
	if ra.ConfigFileName != "" {
		configJsonFileData, err = GetFileContent(ra.ConfigFileName)
		if err != nil {
			color.Error.Println(err)
			os.Exit(-1)
		}
	}
	jsonError := json.Unmarshal(configJsonFileData, &ra.JsonConfig)
	if jsonError != nil {
		color.Error.Println(jsonError)
		os.Exit(-1)
	}
	// 检查保存路径是否能创建成功
	if !CheckFileExists(ra.JsonConfig.SavePath) {
		errMk := os.MkdirAll(ra.JsonConfig.SavePath, 0777)
		if errMk != nil {
			writeLog(ra.JsonConfig.SavePath, "文件夹创建失败")
		}
	}
}

// writeLog 记录日志
func writeLog(filename string, msg ...string) {
	WriteLog(ra.JsonConfig.SavePath, filename, msg...)
}

func getProgramConfig(programName string) (ProgramItem, bool) {
	for _, item := range ra.JsonConfig.Programs {
		if programName == item.ProgramName {
			return item, true
		}
	}
	return ProgramItem{}, false
}
