package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/genkiroid/cert"
	"github.com/go-ping/ping"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"

	"github.com/xos/probe/pkg/utils"
)

func main() {
	// icmp()
	// tcpping()
	// httpWithSSLInfo()
	// sysinfo()
	// cmdExec()
	// resolveIP("ipapi.co", true)
	// resolveIP("ipapi.co", false)
	log.Println(exec.LookPath("powershell.exe"))
	defaultShell := os.Getenv("SHELL")
	if defaultShell == "" {
		defaultShell = "sh"
	}
	cmd := exec.Command(defaultShell)
	cmd.Stdin = os.Stdin
	stdoutReader, err := cmd.StdoutPipe()
	if err != nil {
		println("Terminal StdoutPipe:", err)
		return
	}
	stderrReader, err := cmd.StderrPipe()
	if err != nil {
		println("Terminal StderrPipe: ", err)
		return
	}

	readers := []io.Reader{stdoutReader, stderrReader}
	for i := 0; i < len(readers); i++ {
		go func(j int) {
			data := make([]byte, 2048)
			for {
				count, err := readers[j].Read(data)
				if err != nil {
					panic(err)
				}
				os.Stdout.Write(data[:count])
			}
		}(i)
	}

	cmd.Run()
}

func resolveIP(addr string, ipv6 bool) {
	url := strings.Split(addr, ":")

	dnsServers := []string{"2606:4700:4700::1001", "2001:4860:4860::8844", "2400:3200::1", "2400:3200:baba::1"}
	if !ipv6 {
		dnsServers = []string{"1.0.0.1", "8.8.4.4", "223.5.5.5", "223.6.6.6"}
	}

	log.Println(net.LookupIP(url[0]))
	for i := 0; i < len(dnsServers); i++ {
		dnsServer := dnsServers[i]
		if ipv6 {
			dnsServer = "[" + dnsServer + "]"
		}
		r := &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				d := net.Dialer{
					Timeout: time.Second * 10,
				}
				return d.DialContext(ctx, "udp", dnsServer+":53")
			},
		}
		log.Println(r.LookupIP(context.Background(), "ip", url[0]))
	}
}

func tcpping() {
	start := time.Now()
	conn, err := net.DialTimeout("tcp", "example.com:80", time.Second*10)
	if err != nil {
		panic(err)
	}
	conn.Write([]byte("ping\n"))
	conn.Close()
	fmt.Println(time.Since(start).Microseconds(), float32(time.Since(start).Microseconds())/1000.0)
}

func sysinfo() {
	hi, _ := host.Info()
	var cpuType string
	if hi.VirtualizationSystem != "" {
		cpuType = "Virtual"
	} else {
		cpuType = "Physical"
	}
	cpuModelCount := make(map[string]int)
	ci, _ := cpu.Info()
	for i := 0; i < len(ci); i++ {
		cpuModelCount[ci[i].ModelName]++
	}
	var cpus []string
	for model, count := range cpuModelCount {
		cpus = append(cpus, fmt.Sprintf("%s %d %s Core", model, count, cpuType))
	}
	os.Exit(0)
	// 硬盘信息，不使用的原因是会重复统计 Linux、Mac
	dparts, _ := disk.Partitions(false)
	for _, part := range dparts {
		u, _ := disk.Usage(part.Mountpoint)
		if u != nil {
			log.Printf("%s %d %d", part.Device, u.Total, u.Used)
		}
	}
}

func httpWithSSLInfo() {
	// 跳过 SSL 检查
	transCfg := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	httpClient := &http.Client{Transport: transCfg, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	url := "https://ip.qste.com"
	resp, err := httpClient.Get(url)
	fmt.Println(err, resp)
	// SSL 证书信息获取
	c := cert.NewCert(url[8:])
	fmt.Println(c.Error)
}

func icmp() {
	pinger, err := ping.NewPinger("10.10.10.2")
	if err != nil {
		panic(err) // Blocks until finished.
	}
	pinger.Count = 3000
	pinger.Timeout = 10 * time.Second
	if err = pinger.Run(); err != nil {
		panic(err)
	}
	fmt.Println(pinger.PacketsRecv, float32(pinger.Statistics().AvgRtt.Microseconds())/1000.0)
}

func cmdExec() {
	execFrom, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	var cmd *exec.Cmd
	pg, err := utils.NewProcessExitGroup()
	if err != nil {
		panic(err)
	}
	if utils.IsWindows() {
		cmd = exec.Command("cmd", "/c", os.Args[1])
		// cmd = exec.Command("cmd", "/c", execFrom+"/cmd/playground/example.sh hello asd")
	} else {
		cmd = exec.Command("sh", "-c", execFrom+`/cmd/playground/example.sh hello && \
			echo world!`)
	}
	pg.AddProcess(cmd)
	go func() {
		time.Sleep(time.Second * 10)
		if err = pg.Dispose(); err != nil {
			panic(err)
		}
		fmt.Println("killed")
	}()
	output, err := cmd.Output()
	log.Println("output:", string(output))
	log.Println("err:", err)
}
