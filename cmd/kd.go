package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"runtime"
	"strconv"
	"strings"
	"syscall"

	"github.com/Karmenzind/kd/config"
	"github.com/Karmenzind/kd/internal"
	"github.com/Karmenzind/kd/internal/cache"
	"github.com/Karmenzind/kd/internal/core"
	"github.com/Karmenzind/kd/internal/daemon"
	"github.com/Karmenzind/kd/internal/update"
	"github.com/Karmenzind/kd/logger"
	"github.com/Karmenzind/kd/pkg"
	d "github.com/Karmenzind/kd/pkg/decorate"
	"github.com/kyokomi/emoji/v2"
	"github.com/urfave/cli/v2"
	"go.uber.org/zap"
)

func showPrompt() {
	exename, err := pkg.GetExecutableBasename()
	if err != nil {
		d.EchoFatal(err.Error())
	}
	fmt.Printf(`%[1]s <text>	查单词、词组
%[1]s -h    	查看详细帮助
`, exename)
}

var um = map[string]string{
	"text":            "translate long query `TEXT` with e.g. --text=\"Long time ago\" 翻译长句",
	"nocache":         "don't use cached result 不使用本地词库，查询网络结果",
	"theme":           "choose the color theme for current query 选择颜色主题，仅当前查询生效",
	"init":            "initialize shell completion 初始化部分设置，例如shell的自动补全",
	"server":          "start server foreground 在前台启动服务端",
	"daemon":          "ensure/start the daemon process 启动守护进程",
	"stop":            "stop the daemon process 停止守护进程",
	"update":          "check and update kd client 更新kd的可执行文件",
	"generate-config": "generate config sample 生成配置文件，Linux/Mac默认地址为~/.config/kd.toml，Win为~\\kd.toml",
	"edit-config":     "edit configuration file with the default editor 用默认编辑器打开配置文件",
	"status":          "show running status 展示运行信息",
}

func KillDaemonIfRunning() error {
	p, err := daemon.FindServerProcess()
	var trySysKill bool
	if err == nil {
		if p == nil {
			d.EchoOkay("未发现守护进程，无需停止")
			return nil
		} else if runtime.GOOS != "windows" {
			zap.S().Infof("Found running daemon PID: %d,", p.Pid)
			errSig := p.SendSignal(syscall.SIGINT)
			if errSig != nil {
				zap.S().Warnf("Failed to stop PID %d with syscall.SIGINT: %s", p.Pid, errSig)
				trySysKill = true
			}
		} else {
			trySysKill = true
		}
	} else {
		zap.S().Warnf("[process] Failed to find daemon: %s", err)
		trySysKill = true
	}
	pidStr := strconv.Itoa(int(p.Pid))

	if trySysKill {
		var cmd *exec.Cmd
		switch runtime.GOOS {
		case "windows":
			cmd = exec.Command("taskkill", "/F", "/T", "/PID", pidStr)
			// cmd = exec.Command("taskkill", "/im", "kd", "/T", "/F")
		case "linux":
			cmd = exec.Command("kill", "-9", pidStr)
			// cmd = exec.Command("killall", "kd")
		}
		output, err := cmd.Output()
		zap.S().Infof("Executed '%s'. Output %s", cmd, output)
		if err != nil {
			zap.S().Warnf("Failed to kill daemon with system command. Error: %s", output, err)
		}
	}
	if err == nil {
		zap.S().Info("Terminated daemon process.")
		d.EchoOkay("守护进程已经停止")
	}
	return err
}

//  -----------------------------------------------------------------------------
//  cli flag actions
//  -----------------------------------------------------------------------------

func flagServer(*cli.Context, bool) error {
	err := internal.StartServer()
	if strings.Contains(err.Error(), "address already in use") {
		return fmt.Errorf("端口已经被占用（%s）", err)
	}
	return nil
}

func flagDaemon(*cli.Context, bool) error {
	p, _ := daemon.FindServerProcess()
	if p != nil {
		d.EchoWrong(fmt.Sprintf("已存在运行中的守护进程，PID：%d。请先执行`kd --stop`停止该进程", p.Pid))
		return nil

	}
	err := daemon.StartDaemonProcess()
	if err != nil {
		d.EchoFatal(err.Error())
	}
	return nil
}

func flagStop(*cli.Context, bool) error {
	err := KillDaemonIfRunning()
	if err != nil {
		d.EchoFatal(err.Error())
	}
	return nil
}

func flagUpdate(*cli.Context, bool) error {
	if pkg.GetLinuxDistro() == "arch" {
		d.EchoFine("您在使用ArchLinux，推荐通过AUR安装/升级，更方便省心")
	}
	if pkg.AskYN("Update kd binary file?") {
		emoji.Println(":lightning: Let's update now")
		update.UpdateBinary()
	} else {
		fmt.Println("Canceled.", d.B(d.Green(":)")))
	}
	return nil

}

func flagGenerateConfig(*cli.Context, bool) error {
	conf, err := config.GenerateDefaultConfig()
	if err != nil {
		d.EchoFatal(err.Error())
	}
	d.EchoRun("以下默认配置将会被写入配置文件")
	fmt.Println(conf)
	if pkg.IsPathExists(config.CONFIG_PATH) {
		if !pkg.AskYN(fmt.Sprintf("配置文件%s已经存在，是否覆盖？", config.CONFIG_PATH)) {
			d.EchoFine("已取消")
			return nil
		}
	}
	os.WriteFile(config.CONFIG_PATH, []byte(conf), os.ModePerm)
	d.EchoOkay("已经写入配置文件")
	return err
}

func flagEditConfig(*cli.Context, bool) error {
	var err error
	var cmd *exec.Cmd
	p := config.CONFIG_PATH
	switch runtime.GOOS {
	case "linux":
		for _, k := range []string{"EDITOR", "VISIAL"} {
			if env := os.Getenv(k); env != "" {
				fmt.Println("start editor")
				cmd = exec.Command(env, p)
				break
			}
		}
	case "windows":
		cmd = exec.Command("notepad", p)
	case "darwin":
		cmd = exec.Command("open", "-e", p)
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	err = cmd.Run()
	return err
}

func flagStatus(*cli.Context, bool) error {
	di := internal.GetDaemonInfo()
	d.EchoRun("运行和相关配置信息如下：")
	fmt.Printf("    Daemon端口：%s\n", di.Port)
	fmt.Printf("    Daemon PID：%d\n", di.PID)
	fmt.Printf("    配置文件地址：%s\n", config.CONFIG_PATH)
	fmt.Printf("    数据文件目录：%s\n", cache.CACHE_ROOT_PATH)
	return nil
}

func main() {
	config.InitConfig()
	cfg := config.Cfg
	d.ApplyConfig(cfg.EnableEmoji)
	if u, _ := user.Current(); u.Username == "root" {
		d.EchoWrong("不支持Root用户")
		os.Exit(1)
	}
	if cfg.Logging.Enable {
		l, err := logger.InitLogger(&cfg.Logging)
		if err != nil {
			d.EchoFatal(err.Error())
		}
		defer l.Sync()
	}

	err := cache.InitDB()
	if err != nil {
		d.EchoFatal(err.Error())
	}
	defer cache.LiteDB.Close()
	defer core.WG.Wait()
	// emoji.Println(":beer: Beer!!!")
	// pizzaMessage := emoji.Sprint("I like a :pizza: and :sushi:!!")
	// fmt.Println(pizzaMessage)

	app := &cli.App{
		Suggest:         true, // XXX
		Name:            "kd",
		Version:         "v0.0.1",
		Usage:           "A crystal clean command-line dictionary.",
		HideHelpCommand: true,
		// EnableBashCompletion: true,
		// EnableShellCompletion: true,

		// Authors: []*cli.Author{{Name: "kmz", Email: "valesail7@gmail.com"}},
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "text", Aliases: []string{"t"}, Hidden: true, Usage: um["text"]},
			&cli.BoolFlag{Name: "nocache", Aliases: []string{"n"}, DisableDefaultText: true, Usage: um["nocache"]},
			&cli.StringFlag{Name: "theme", Aliases: []string{"T"}, DefaultText: "temp", Usage: um["theme"]},

			// BoolFlags as commands
			&cli.BoolFlag{Name: "init", DisableDefaultText: true, Hidden: true, Usage: um["init"]},
			&cli.BoolFlag{Name: "server", DisableDefaultText: true, Action: flagServer, Usage: um["server"]},
			&cli.BoolFlag{Name: "daemon", DisableDefaultText: true, Action: flagDaemon, Usage: um["daemon"]},
			&cli.BoolFlag{Name: "stop", DisableDefaultText: true, Hidden: true, Action: flagStop, Usage: um["stop"]},
			&cli.BoolFlag{Name: "update", DisableDefaultText: true, Action: flagUpdate, Usage: um["update"]},
			&cli.BoolFlag{Name: "generate-config", DisableDefaultText: true, Action: flagGenerateConfig, Usage: um["generate-config"]},
			&cli.BoolFlag{Name: "edit-config", DisableDefaultText: true, Action: flagEditConfig, Usage: um["edit-config"]},
			&cli.BoolFlag{Name: "status", DisableDefaultText: true, Action: flagStatus, Usage: um["status"]},
		},
		Action: func(cCtx *cli.Context) error {
			// 除了--text外，其他的BoolFlag都当subcommand用
			for _, flag := range []string{"init", "server", "daemon", "stop", "update", "generate-config", "edit-config", "status"} {
				if cCtx.Bool(flag) {
					return nil
				}
			}

			if cfg.FileExists && cfg.ModTime > internal.GetDaemonInfo().StartTime {
				d.EchoWarn("检测到配置文件发生修改，正在重启守护进程")
				flagStop(cCtx, true)
				flagDaemon(cCtx, true)
			}

			if cCtx.String("theme") != "" {
				cfg.Theme = cCtx.String("theme")
			}
			d.ApplyTheme(cfg.Theme)

			if cCtx.Args().Len() > 0 {
				zap.S().Debugf("Recieved Arguments (len: %d): %+v \n", cCtx.Args().Len(), cCtx.Args().Slice())
				// emoji.Printf(":eyes: Arguments are: %+v \n", cCtx.Args().Slice())
				// emoji.Printf(":eyes: Flat --update  %+v \n", cCtx.Bool("update"))
				// emoji.Printf(":eyes: Flat --nocache  %+v \n", cCtx.Bool("nocache"))
				// emoji.Printf(":eyes: flags are: %+v \n", cCtx.App.VisibleFlags)
				// emoji.Printf("Test emoji:\n:accept: :inbox_tray: :information: :us: :uk:  🗣  :lips: :eyes: :balloon: \n")

				qstr := strings.Join(cCtx.Args().Slice(), " ")

				r, err := internal.Query(qstr, cCtx.Bool("nocache"))
				if cfg.FreqAlert {
					if h := <-r.History; h > 3 {
						d.EchoWarn(fmt.Sprintf("本月第%d次查询`%s`", h, r.Query))
					}
				}
				if err == nil {
					if r.Found {
						err = pkg.OutputResult(r.PrettyFormat(cfg.EnglishOnly), cfg.Paging, cfg.PagerCommand, cfg.ClearScreen)
						if err != nil {
							d.EchoFatal(err.Error())
						}
					} else {
						if r.Prompt != "" {
							d.EchoWrong(r.Prompt)
						} else {
							fmt.Println("Not found", d.Yellow(":("))
						}
					}
				} else {
					d.EchoError(err.Error())
					zap.S().Errorf("%+v\n", err)
				}
			} else {
				showPrompt()
			}
			return nil
		},
	}

	if err := app.Run(os.Args); err != nil {
		zap.S().Errorf("APP stopped: %s", err)
		d.EchoError(err.Error())
	}
}
