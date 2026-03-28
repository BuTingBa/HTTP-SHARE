package main

import (
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mdp/qrterminal/v3"
)

//go:embed index.html
var htmlTmpl string

//go:embed logo.png
var logoData []byte

type Config struct {
	Port     string `json:"port"`
	Password string `json:"password"`
	Host     string `json:"host"`
	Mode     string `json:"mode"`
}

type FileInfo struct {
	Name, Size, ModTime, URL string
	IsDir                    bool
}
type Breadcrumb struct{ Name, Path string }
type PageData struct {
	NeedsLogin, LoginError, IsFile          bool
	FileName, Size, CurrentPath, ParentPath string
	Files                                   []FileInfo
	Breadcrumbs                             []Breadcrumb
}

const cookieName = "hs_auth_token"

func main() {
	// 1. 预处理子命令
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "help", "-h", "--help":
			printHelp()
			return
		case "v", "-v", "--version":
			printVersion()
			return
		case "config":
			handleConfigCmd()
			return
		}
	}

	// 2. 主服务参数解析
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	inputDir := fs.String("i", ".", "输入文件或目录路径")
	argPort := fs.String("p", "", "服务端口")
	argHost := fs.String("h", "", "自定义外网IP或域名")
	argPass := fs.String("pass", "", "访问密码")
	argMode := fs.String("m", "", "网络模式: lan, ipv6, custom")

	fs.Usage = printHelp
	fs.Parse(os.Args[1:])

	// 3. 读取本地配置
	cfg := loadConfig()

	// 4. 优先级合并
	finalPort := override(*argPort, cfg.Port, "1120")
	finalPass := override(*argPass, cfg.Password, "")
	finalHost := override(*argHost, cfg.Host, "")
	finalMode := override(*argMode, cfg.Mode, "lan")

	// 5. 校验路径
	absTarget, err := filepath.Abs(*inputDir)
	if err != nil {
		log.Fatalf("无法获取绝对路径: %v", err)
	}
	info, err := os.Stat(absTarget)
	if err != nil {
		log.Fatalf("路径不存在: %v", err)
	}

	// 6. 端口占用检测与自增机制
	portInt, err := strconv.Atoi(finalPort)
	if err != nil {
		portInt = 1120
	}
	var listener net.Listener
	for {
		l, err := net.Listen("tcp", fmt.Sprintf(":%d", portInt))
		if err == nil {
			listener = l
			finalPort = strconv.Itoa(portInt) // 更新为最终成功绑定的端口
			break
		}
		// 端口被占用，自增尝试
		portInt++
	}

	// 7. 网络探测与 URL 生成
	ipv4, ipv6List := getNetworkIPs()
	var primaryURL string

	switch finalMode {
	case "ipv6":
		if len(ipv6List) > 0 {
			primaryURL = fmt.Sprintf("http://[%s]:%s", ipv6List[0], finalPort)
		} else {
			primaryURL = fmt.Sprintf("http://%s:%s", ipv4, finalPort)
		}
	case "custom":
		if finalHost != "" {
			primaryURL = fmt.Sprintf("http://%s:%s", finalHost, finalPort)
		} else {
			primaryURL = fmt.Sprintf("http://%s:%s", ipv4, finalPort)
		}
	default: // lan
		primaryURL = fmt.Sprintf("http://%s:%s", ipv4, finalPort)
	}

	// 8. 终端输出
	fmt.Printf("\n🚀 HTTP SHARE 服务已启动!\n")
	fmt.Printf("=========================================\n")
	if ipv4 != "" {
		fmt.Printf("📍 本地 IPv4 : http://%s:%s\n", ipv4, finalPort)
	}
	for _, v6 := range ipv6List {
		fmt.Printf("🌍 公网 IPv6 : http://[%s]:%s\n", v6, finalPort)
	}
	if finalHost != "" {
		fmt.Printf("🔗 自定义地址: http://%s:%s\n", finalHost, finalPort)
	}
	if finalPass != "" {
		fmt.Printf("🔒 访问密码  : %s\n", finalPass)
	}
	fmt.Printf("=========================================\n")
	fmt.Printf("📱 扫码访问 [%s 模式]:\n", strings.ToUpper(finalMode))

	qrterminal.GenerateHalfBlock(primaryURL, qrterminal.L, os.Stdout)
	fmt.Println()

	// 9. 启动 Web 服务
	tmpl := template.Must(template.New("index").Parse(htmlTmpl))

	mux := http.NewServeMux()
	
	// 处理静态 Logo 资源
	mux.HandleFunc("/logo.png", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "public, max-age=86400")
		w.Write(logoData)
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if finalPass != "" {
			cookie, err := r.Cookie(cookieName)
			if err != nil || cookie.Value != finalPass {
				tmpl.Execute(w, PageData{NeedsLogin: true})
				return
			}
		}
		handleShare(w, r, absTarget, info.IsDir(), tmpl)
	})

	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		if r.FormValue("password") == finalPass {
			http.SetCookie(w, &http.Cookie{Name: cookieName, Value: finalPass, MaxAge: 86400, Path: "/"})
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		tmpl.Execute(w, PageData{NeedsLogin: true, LoginError: true})
	})

	// 使用已成功绑定的 listener 启动服务
	log.Fatal(http.Serve(listener, mux))
}

// ---------------------------------------------------------
// 辅助函数与命令系统
// ---------------------------------------------------------

func printHelp() {
	fmt.Println("HTTP SHARE - 极简本地文件共享工具")
	fmt.Println("\n用法:")
	fmt.Println("  hs [命令] [参数]")
	fmt.Println("\n主命令 (直接启动服务):")
	fmt.Println("  hs -i <路径> [-p 端口] [-pass 密码] [-m 模式] [-h 域名]")
	fmt.Println("    -i string    输入文件或目录路径 (必须项或默认当前目录)")
	fmt.Println("    -p string    指定服务端口 (默认 1120)")
	fmt.Println("    -pass string 开启密码保护")
	fmt.Println("    -m string    网络模式优先渲染二维码: lan, ipv6, custom (默认 lan)")
	fmt.Println("    -h string    自定义外网 IP 或域名 (用于 custom 模式)")
	fmt.Println("\n配置命令 (持久化修改默认设置):")
	fmt.Println("  hs config [参数]")
	fmt.Println("    hs config -p 8080 -pass 123456 -m ipv6 -h share.com")
	fmt.Println("\n其他命令:")
	fmt.Println("  hs help        显示此帮助信息")
	fmt.Println("  hs v        显示版本信息")
}

func printVersion(){
	fmt.Println("HTTP SHARE - 极简本地文件共享工具")
	fmt.Println("\n作者：BuTing <my@buting.cc>")
	fmt.Println("\n版本号：1.0.0")
}

func handleConfigCmd() {
	fs := flag.NewFlagSet("config", flag.ExitOnError)
	port := fs.String("p", "", "默认端口")
	pass := fs.String("pass", "", "默认密码")
	host := fs.String("h", "", "默认域名")
	mode := fs.String("m", "", "默认模式 (lan/ipv6/custom)")
	
	fs.Parse(os.Args[2:])
	
	cfg := loadConfig()
	updated := false

	if *port != "" { cfg.Port = *port; updated = true }
	if *pass != "" { cfg.Password = *pass; updated = true }
	if *host != "" { cfg.Host = *host; updated = true }
	if *mode != "" { cfg.Mode = *mode; updated = true }

	if !updated {
		fmt.Println("当前配置信息:")
		d, _ := json.MarshalIndent(cfg, "", "  ")
		fmt.Println(string(d))
		fmt.Println("\n如需修改，请附加参数，例如: hs config -p 8888")
		return
	}

	saveConfig(cfg)
	fmt.Println("✅ 配置已成功更新并保存至 config.json")
}

func getConfigPath() string {
	exePath, _ := os.Executable()
	return filepath.Join(filepath.Dir(exePath), "config.json")
}

func loadConfig() Config {
	var cfg Config
	if fileData, err := ioutil.ReadFile(getConfigPath()); err == nil {
		json.Unmarshal(fileData, &cfg)
	}
	return cfg
}

func saveConfig(cfg Config) {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err == nil {
		ioutil.WriteFile(getConfigPath(), data, 0644)
	}
}

func override(arg, cfg, def string) string {
	if arg != "" { return arg }
	if cfg != "" { return cfg }
	return def
}

func handleShare(w http.ResponseWriter, r *http.Request, root string, isDir bool, tmpl *template.Template) {
	if !isDir {
		if r.URL.Query().Get("download") == "1" {
			http.ServeFile(w, r, root)
			return
		}
		info, _ := os.Stat(root)
		tmpl.Execute(w, PageData{IsFile: true, FileName: filepath.Base(root), Size: formatSize(info.Size())})
		return
	}

	reqPath := filepath.Clean(r.URL.Path)
	fullPath := filepath.Join(root, reqPath)
	if !strings.HasPrefix(fullPath, root) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	stat, err := os.Stat(fullPath)
	if err != nil || !stat.IsDir() {
		if err == nil && !stat.IsDir() {
			http.ServeFile(w, r, fullPath)
			return
		}
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	entries, _ := os.ReadDir(fullPath)
	var files []FileInfo
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil { continue }
		fileURL := filepath.ToSlash(filepath.Join(reqPath, entry.Name()))
		if fileURL[0] != '/' {
			fileURL = "/" + fileURL
		}
		files = append(files, FileInfo{Name: entry.Name(), IsDir: entry.IsDir(), Size: formatSize(info.Size()), ModTime: info.ModTime().Format("2006-01-02 15:04"), URL: fileURL})
	}

	var breadcrumbs []Breadcrumb
	currentBuild := ""
	for _, part := range strings.Split(strings.Trim(reqPath, "/"), "/") {
		if part == "" { continue }
		currentBuild += "/" + part
		breadcrumbs = append(breadcrumbs, Breadcrumb{Name: part, Path: currentBuild})
	}

	parentPath := "/"
	if reqPath != "/" && reqPath != "." {
		parentPath = filepath.ToSlash(filepath.Dir(reqPath))
		if parentPath[0] != '/' {
			parentPath = "/" + parentPath
		}
	}

	tmpl.Execute(w, PageData{IsFile: false, Files: files, Breadcrumbs: breadcrumbs, CurrentPath: reqPath, ParentPath: parentPath})
}

func formatSize(bytes int64) string {
	if bytes < 1024 {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(1024), 0
	for n := bytes / 1024; n >= 1024; n /= 1024 {
		div *= 1024
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func getNetworkIPs() (ipv4 string, ipv6 []string) {
	addrs, _ := net.InterfaceAddrs()
	for _, address := range addrs {
		if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			ip := ipnet.IP
			if ip.To4() != nil {
				if ipv4 == "" || strings.HasPrefix(ip.String(), "192.168.") || strings.HasPrefix(ip.String(), "10.") {
					ipv4 = ip.String()
				}
			} else if ip.To16() != nil && !ip.IsLinkLocalUnicast() && !ip.IsLoopback() && !ip.IsMulticast() {
				ipv6 = append(ipv6, ip.String())
			}
		}
	}
	if ipv4 == "" {
		ipv4 = "127.0.0.1"
	}
	return
}