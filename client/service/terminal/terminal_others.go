//go:build !windows

package terminal

import (
	"Spark/client/common"
	"Spark/modules"
	"Spark/utils"
	"encoding/hex"
	"github.com/creack/pty"
	"os"
	"os/exec"
	"reflect"
	"time"
)

type terminal struct {
	escape   bool
	lastPack int64
	event    string
	pty      *os.File
	cmd      *exec.Cmd
}

var defaultShell = ``

func init() {
	go healthCheck()
}

func InitTerminal(pack modules.Packet) error {
	// try to get shell
	// if shell is not found or unavailable, then fallback to `sh`
	cmd := exec.Command(getTerminal(false))
	ptySession, err := pty.Start(cmd)
	if err != nil {
		defaultShell = getTerminal(true)
		return err
	}
	termSession := &terminal{
		cmd:      cmd,
		pty:      ptySession,
		event:    pack.Event,
		lastPack: utils.Unix,
		escape:   false,
	}
	terminals.Set(pack.Data[`terminal`].(string), termSession)
	go func() {
		for !termSession.escape {
			buffer := make([]byte, 512)
			n, err := ptySession.Read(buffer)
			buffer = buffer[:n]
			common.WSConn.SendCallback(modules.Packet{Act: `TERMINAL_OUTPUT`, Data: map[string]any{
				`output`: hex.EncodeToString(buffer),
			}}, pack)
			termSession.lastPack = utils.Unix
			if err != nil {
				if !termSession.escape {
					termSession.escape = true
					doKillTerminal(termSession)
				}
				common.WSConn.SendCallback(modules.Packet{Act: `TERMINAL_QUIT`}, pack)
				break
			}
		}
	}()

	return nil
}

func InputTerminal(pack modules.Packet) error {
	val, ok := pack.GetData(`input`, reflect.String)
	if !ok {
		return errDataNotFound
	}
	data, err := hex.DecodeString(val.(string))
	if err != nil {
		return errDataInvalid
	}

	val, ok = pack.GetData(`terminal`, reflect.String)
	if !ok {
		return errUUIDNotFound
	}
	termUUID := val.(string)
	val, ok = terminals.Get(termUUID)
	if !ok {
		common.WSConn.SendCallback(modules.Packet{Act: `TERMINAL_QUIT`, Msg: `${i18n|TERMINAL.SESSION_CLOSED}`}, pack)
		return nil
	}
	terminal := val.(*terminal)
	terminal.pty.Write(data)
	terminal.lastPack = utils.Unix
	return nil
}

func ResizeTerminal(pack modules.Packet) error {
	val, ok := pack.GetData(`width`, reflect.Float64)
	if !ok {
		return errDataInvalid
	}
	width := val.(float64)
	val, ok = pack.GetData(`height`, reflect.Float64)
	if !ok {
		return errDataInvalid
	}
	height := val.(float64)

	val, ok = pack.GetData(`terminal`, reflect.String)
	if !ok {
		return errUUIDNotFound
	}
	termUUID := val.(string)
	val, ok = terminals.Get(termUUID)
	if !ok {
		common.WSConn.SendCallback(modules.Packet{Act: `TERMINAL_QUIT`, Msg: `${i18n|TERMINAL.SESSION_CLOSED}`}, pack)
		return nil
	}
	terminal := val.(*terminal)
	pty.Setsize(terminal.pty, &pty.Winsize{
		Rows: uint16(height),
		Cols: uint16(width),
	})
	return nil
}

func KillTerminal(pack modules.Packet) error {
	val, ok := pack.GetData(`terminal`, reflect.String)
	if !ok {
		return errUUIDNotFound
	}
	termUUID := val.(string)
	val, ok = terminals.Get(termUUID)
	if !ok {
		common.WSConn.SendCallback(modules.Packet{Act: `TERMINAL_QUIT`, Msg: `${i18n|TERMINAL.SESSION_CLOSED}`}, pack)
		return nil
	}
	terminal := val.(*terminal)
	terminals.Remove(termUUID)
	doKillTerminal(terminal)
	return nil
}

func PingTerminal(pack modules.Packet) {
	var termUUID string
	var termSession *terminal
	if val, ok := pack.GetData(`terminal`, reflect.String); !ok {
		return
	} else {
		termUUID = val.(string)
	}
	if val, ok := terminals.Get(termUUID); !ok {
		return
	} else {
		termSession = val.(*terminal)
		termSession.lastPack = utils.Unix
	}
}

func doKillTerminal(terminal *terminal) {
	terminal.escape = true
	if terminal.pty != nil {
		terminal.pty.Close()
	}
	if terminal.cmd.Process != nil {
		terminal.cmd.Process.Kill()
		terminal.cmd.Process.Wait()
		terminal.cmd.Process.Release()
		terminal.cmd.Process = nil
	}
}

func getTerminal(sh bool) string {
	shellTable := []string{`zsh`, `bash`, `sh`}
	if sh {
		shPath, err := exec.LookPath(`sh`)
		if err != nil {
			return `sh`
		}
		return shPath
	} else if len(defaultShell) > 0 {
		return defaultShell
	}
	for i := 0; i < len(shellTable); i++ {
		shellPath, err := exec.LookPath(shellTable[i])
		if err == nil {
			defaultShell = shellPath
			return shellPath
		}
	}
	return `sh`
}

func healthCheck() {
	const MaxInterval = 300
	for now := range time.NewTicker(30 * time.Second).C {
		timestamp := now.Unix()
		// stores sessions to be disconnected
		queue := make([]string, 0)
		terminals.IterCb(func(uuid string, t any) bool {
			termSession := t.(*terminal)
			if timestamp-termSession.lastPack > MaxInterval {
				queue = append(queue, uuid)
				doKillTerminal(termSession)
			}
			return true
		})
		for i := 0; i < len(queue); i++ {
			terminals.Remove(queue[i])
		}
	}
}
