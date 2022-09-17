//go:build !windows

package terminal

import (
	"Spark/client/common"
	"Spark/modules"
	"encoding/hex"
	"errors"
	"github.com/creack/pty"
	"os"
	"os/exec"
	"reflect"
	"time"
)

type terminal struct {
	lastPack int64
	event    string
	pty      *os.File
}

func init() {
	go healthCheck()
}

func InitTerminal(pack modules.Packet) error {
	cmd := exec.Command(getTerminal())
	ptySession, err := pty.Start(cmd)
	if err != nil {
		return err
	}
	termSession := &terminal{
		pty:      ptySession,
		event:    pack.Event,
		lastPack: common.Unix,
	}
	terminals.Set(pack.Data[`terminal`].(string), termSession)
	go func() {
		for {
			buffer := make([]byte, 512)
			n, err := ptySession.Read(buffer)
			buffer = buffer[:n]
			common.WSConn.SendCallback(modules.Packet{Act: `outputTerminal`, Data: map[string]interface{}{
				`output`: hex.EncodeToString(buffer),
			}}, pack)
			termSession.lastPack = common.Unix
			if err != nil {
				common.WSConn.SendCallback(modules.Packet{Act: `quitTerminal`}, pack)
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
		common.WSConn.SendCallback(modules.Packet{Act: `quitTerminal`, Msg: `${i18n|terminalSessionClosed}`}, pack)
		return nil
	}
	terminal := val.(*terminal)
	terminal.pty.Write(data)
	terminal.lastPack = common.Unix
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
		common.WSConn.SendCallback(modules.Packet{Act: `quitTerminal`, Msg: `${i18n|terminalSessionClosed}`}, pack)
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
		common.WSConn.SendCallback(modules.Packet{Act: `quitTerminal`, Msg: `${i18n|terminalSessionClosed}`}, pack)
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
		termSession.lastPack = common.Unix
	}
}

func doKillTerminal(terminal *terminal) {
	if terminal.pty != nil {
		terminal.pty.Close()
	}
}

func getTerminal() string {
	sh := []string{`/bin/zsh`, `/bin/bash`, `/bin/sh`}
	for i := 0; i < len(sh); i++ {
		_, err := os.Stat(sh[i])
		if !errors.Is(err, os.ErrNotExist) {
			return sh[i]
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
		terminals.IterCb(func(uuid string, t interface{}) bool {
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
