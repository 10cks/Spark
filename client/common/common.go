package common

import (
	"Spark/client/config"
	"Spark/modules"
	"Spark/utils"
	"encoding/hex"
	"errors"
	ws "github.com/gorilla/websocket"
	"github.com/imroc/req/v3"
	"sync"
	"time"
)

type Conn struct {
	*ws.Conn
	secret    []byte
	secretHex string
}

var WSConn *Conn
var Mutex = &sync.Mutex{}
var HTTP = CreateClient()

const MaxMessageSize = 32768 + 1024

func CreateConn(wsConn *ws.Conn, secret []byte) *Conn {
	return &Conn{
		Conn:      wsConn,
		secret:    secret,
		secretHex: hex.EncodeToString(secret),
	}
}

func CreateClient() *req.Client {
	return req.C().SetUserAgent(`SPARK COMMIT: ` + config.COMMIT)
}

func (wsConn *Conn) SendData(data []byte) error {
	Mutex.Lock()
	defer Mutex.Unlock()
	if WSConn == nil {
		return errors.New(`${i18n|wsClosed}`)
	}
	wsConn.SetWriteDeadline(Now.Add(5 * time.Second))
	defer wsConn.SetWriteDeadline(time.Time{})
	return wsConn.WriteMessage(ws.BinaryMessage, data)
}

func (wsConn *Conn) SendPack(pack interface{}) error {
	Mutex.Lock()
	defer Mutex.Unlock()
	data, err := utils.JSON.Marshal(pack)
	if err != nil {
		return err
	}
	data, err = utils.Encrypt(data, wsConn.secret)
	if err != nil {
		return err
	}
	if len(data) > MaxMessageSize {
		_, err = HTTP.R().
			SetBody(data).
			SetHeader(`Secret`, wsConn.secretHex).
			Send(`POST`, config.GetBaseURL(false)+`/ws`)
		return err
	}
	if WSConn == nil {
		return errors.New(`${i18n|wsClosed}`)
	}
	wsConn.SetWriteDeadline(Now.Add(5 * time.Second))
	defer wsConn.SetWriteDeadline(time.Time{})
	return wsConn.WriteMessage(ws.BinaryMessage, data)
}

func (wsConn *Conn) SendCallback(pack, prev modules.Packet) error {
	if len(prev.Event) > 0 {
		pack.Event = prev.Event
	}
	return wsConn.SendPack(pack)
}

func (wsConn *Conn) GetSecret() []byte {
	return wsConn.secret
}

func (wsConn *Conn) GetSecretHex() string {
	return wsConn.secretHex
}
