package service

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

func StartAccountClosureTask() {
	if !common.IsMasterNode || model.DB == nil {
		return
	}
	mode := os.Getenv("ACCOUNT_AUTO_CLOSURE_MODE")
	switch mode {
	case "off":
		return
	case "preview", "enforce":
	case "":
		mode = "enforce"
	default:
		common.SysError("invalid ACCOUNT_AUTO_CLOSURE_MODE; account closure is off")
		return
	}
	go func() {
		interval := time.Minute
		if mode == "preview" {
			interval = time.Hour
		}
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			now := time.Now()
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			if mode == "preview" {
				count, err := model.PreviewAccountClosures(ctx, now)
				if err != nil {
					common.SysError("automatic account closure preview: " + err.Error())
				} else {
					common.SysLog(fmt.Sprintf("automatic account closure preview: %d eligible accounts", count))
				}
			} else {
				ids, err := model.CloseExpiredAccounts(ctx, now, 200)
				for _, id := range ids {
					common.SysLog(fmt.Sprintf("automatically closed account %d: balance threshold was not reached in 72 hours", id))
				}
				if err != nil {
					common.SysError("automatic account closure: " + err.Error())
				}
			}
			cancel()
			<-ticker.C
		}
	}()
}
