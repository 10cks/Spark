package handler

import (
	"Spark/modules"
	"Spark/server/common"
	"Spark/utils"
	"Spark/utils/melody"
	"fmt"
	"github.com/gin-gonic/gin"
	"io/ioutil"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
)

// removeDeviceFile will try to get send a packet to
// client and let it upload the file specified.
func removeDeviceFile(ctx *gin.Context) {
	var form struct {
		File string `json:"file" yaml:"file" form:"file" binding:"required"`
	}
	target, ok := checkForm(ctx, &form)
	if !ok {
		return
	}
	trigger := utils.GetStrUUID()
	common.SendPackUUID(modules.Packet{Code: 0, Act: `removeFile`, Data: gin.H{`file`: form.File}, Event: trigger}, target)
	ok = common.AddEventOnce(func(p modules.Packet, _ *melody.Session) {
		if p.Code != 0 {
			ctx.JSON(http.StatusInternalServerError, modules.Packet{Code: 1, Msg: p.Msg})
		} else {
			ctx.JSON(http.StatusOK, modules.Packet{Code: 0})
		}
	}, target, trigger, 5*time.Second)
	if !ok {
		ctx.JSON(http.StatusGatewayTimeout, modules.Packet{Code: 1, Msg: `${i18n|responseTimeout}`})
	}
}

// listDeviceFiles will list files on remote client
func listDeviceFiles(ctx *gin.Context) {
	var form struct {
		Path string `json:"path" yaml:"path" form:"path" binding:"required"`
	}
	target, ok := checkForm(ctx, &form)
	if !ok {
		return
	}
	trigger := utils.GetStrUUID()
	common.SendPackUUID(modules.Packet{Act: `listFiles`, Data: gin.H{`path`: form.Path}, Event: trigger}, target)
	ok = common.AddEventOnce(func(p modules.Packet, _ *melody.Session) {
		if p.Code != 0 {
			ctx.JSON(http.StatusInternalServerError, modules.Packet{Code: 1, Msg: p.Msg})
		} else {
			ctx.JSON(http.StatusOK, modules.Packet{Code: 0, Data: p.Data})
		}
	}, target, trigger, 5*time.Second)
	if !ok {
		ctx.JSON(http.StatusGatewayTimeout, modules.Packet{Code: 1, Msg: `${i18n|responseTimeout}`})
	}
}

// getDeviceFile will try to get send a packet to
// client and let it upload the file specified.
func getDeviceFile(ctx *gin.Context) {
	var form struct {
		File string `json:"file" yaml:"file" form:"file" binding:"required"`
	}
	target, ok := checkForm(ctx, &form)
	if !ok {
		return
	}
	trigger := utils.GetStrUUID()
	var rangeStart, rangeEnd int64
	var err error
	partial := false
	{
		command := gin.H{`file`: form.File}
		rangeHeader := ctx.GetHeader(`Range`)
		if len(rangeHeader) > 6 {
			if rangeHeader[:6] != `bytes=` {
				ctx.Status(http.StatusRequestedRangeNotSatisfiable)
				return
			}
			rangeHeader = strings.TrimSpace(rangeHeader[6:])
			rangesList := strings.Split(rangeHeader, `,`)
			if len(rangesList) > 1 {
				ctx.Status(http.StatusRequestedRangeNotSatisfiable)
				return
			}
			r := strings.Split(rangesList[0], `-`)
			rangeStart, err = strconv.ParseInt(r[0], 10, 64)
			if err != nil {
				ctx.Status(http.StatusRequestedRangeNotSatisfiable)
				return
			}
			if len(r[1]) > 0 {
				rangeEnd, err = strconv.ParseInt(r[1], 10, 64)
				if err != nil {
					ctx.Status(http.StatusRequestedRangeNotSatisfiable)
					return
				}
				if rangeEnd < rangeStart {
					ctx.Status(http.StatusRequestedRangeNotSatisfiable)
					return
				}
				command[`end`] = rangeEnd
			}
			command[`start`] = rangeStart
			partial = true
		}
		common.SendPackUUID(modules.Packet{Code: 0, Act: `uploadFile`, Data: command, Event: trigger}, target)
	}

	wait := make(chan bool)
	called := false
	common.AddEvent(func(p modules.Packet, _ *melody.Session) {
		called = true
		common.RemoveEvent(trigger)
		if p.Code != 0 {
			wait <- false
			ctx.JSON(http.StatusInternalServerError, modules.Packet{Code: 1, Msg: p.Msg})
			return
		} else {
			val, ok := p.Data[`request`]
			if !ok {
				wait <- false
				ctx.JSON(http.StatusInternalServerError, modules.Packet{Code: 1, Msg: `${i18n|fileUploadFailed}`})
				return
			}
			req, ok := val.(*http.Request)
			if !ok || req == nil || req.Body == nil {
				wait <- false
				ctx.JSON(http.StatusInternalServerError, modules.Packet{Code: 1, Msg: `${i18n|fileUploadFailed}`})
				return
			}

			if req.ContentLength > 0 {
				ctx.Header(`Content-Length`, strconv.FormatInt(req.ContentLength, 10))
			}
			ctx.Header(`Accept-Ranges`, `bytes`)
			ctx.Header(`Content-Transfer-Encoding`, `binary`)
			ctx.Header(`Content-Type`, `application/octet-stream`)
			filename := ctx.GetHeader(`FileName`)
			if len(filename) == 0 {
				filename = path.Base(strings.ReplaceAll(form.File, `\`, `/`))
			}
			filename = url.PathEscape(filename)
			ctx.Header(`Content-Disposition`, `attachment; filename* = UTF-8''`+filename+`;`)

			if partial {
				if rangeEnd == 0 {
					rangeEnd, err = strconv.ParseInt(req.Header.Get(`FileSize`), 10, 64)
					if err == nil {
						ctx.Header(`Content-Range`, fmt.Sprintf(`bytes %d-%d/%d`, rangeStart, rangeEnd-1, rangeEnd))
					}
				} else {
					ctx.Header(`Content-Range`, fmt.Sprintf(`bytes %d-%d/%v`, rangeStart, rangeEnd, req.Header.Get(`FileSize`)))
				}
				ctx.Status(http.StatusPartialContent)
			} else {
				ctx.Status(http.StatusOK)
			}

			for {
				buffer := make([]byte, 8192)
				n, err := req.Body.Read(buffer)
				buffer = buffer[:n]
				ctx.Writer.Write(buffer)
				ctx.Writer.Flush()
				if n == 0 || err != nil {
					wait <- false
					break
				}
			}
		}
	}, target, trigger)
	select {
	case <-wait:
	case <-time.After(5 * time.Second):
		if !called {
			common.RemoveEvent(trigger)
			ctx.JSON(http.StatusGatewayTimeout, modules.Packet{Code: 1, Msg: `${i18n|responseTimeout}`})
		} else {
			<-wait
		}
	}
}

// putDeviceFile will be called by client.
// It will transfer binary stream from client to browser.
func putDeviceFile(ctx *gin.Context) {
	original := ctx.Request.Body
	ctx.Request.Body = ioutil.NopCloser(ctx.Request.Body)

	errMsg := ctx.GetHeader(`Error`)
	trigger := ctx.GetHeader(`Trigger`)
	if len(trigger) == 0 {
		original.Close()
		ctx.JSON(http.StatusBadRequest, modules.Packet{Code: -1, Msg: `${i18n|invalidParameter}`})
		return
	}
	if len(errMsg) > 0 {
		common.CallEvent(modules.Packet{
			Code:  1,
			Msg:   fmt.Sprintf(`${i18n|fileUploadFailed}: %v`, errMsg),
			Event: trigger,
		}, nil)
		original.Close()
		ctx.JSON(http.StatusOK, modules.Packet{Code: 0})
		return
	}
	common.CallEvent(modules.Packet{
		Code: 0,
		Data: map[string]interface{}{
			`request`: ctx.Request,
		},
		Event: trigger,
	}, nil)
	original.Close()
	ctx.JSON(http.StatusOK, modules.Packet{Code: 0})
}
