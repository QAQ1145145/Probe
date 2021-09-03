package rpc

import (
	"context"
	"fmt"
	"time"

	"github.com/xos/probe/model"
	"github.com/xos/probe/pkg/utils"
	pb "github.com/xos/probe/proto"
	"github.com/xos/probe/service/dao"
)

type ProbeHandler struct {
	Auth *AuthHandler
}

func (s *ProbeHandler) ReportTask(c context.Context, r *pb.TaskResult) (*pb.Receipt, error) {
	var err error
	var clientID uint64
	if clientID, err = s.Auth.Check(c); err != nil {
		return nil, err
	}
	if r.GetType() != model.TaskTypeCommand {
		dao.ServiceSentinelShared.Dispatch(dao.ReportData{
			Data:     r,
			Reporter: clientID,
		})
	} else {
		// 处理上报的计划任务
		dao.CronLock.RLock()
		defer dao.CronLock.RUnlock()
		cr := dao.Crons[r.GetId()]
		if cr != nil {
			dao.ServerLock.RLock()
			defer dao.ServerLock.RUnlock()
			if cr.PushSuccessful && r.GetSuccessful() {
				dao.SendNotification(fmt.Sprintf("#探针通知" + "\n" + "成功计划任务：%s 。" + "\n" + "服务器：%s，日志：\n%s", cr.Name, dao.ServerList[clientID].Name, r.GetData()), false)
			}
			if !r.GetSuccessful() {
				dao.SendNotification(fmt.Sprintf("#探针通知" + "\n" + "失败计划任务：%s 。" + "\n" + "服务器：%s，日志：\n%s", cr.Name, dao.ServerList[clientID].Name, r.GetData()), false)
			}
			dao.DB.Model(cr).Updates(model.Cron{
				LastExecutedAt: time.Now().Add(time.Second * -1 * time.Duration(r.GetDelay())),
				LastResult:     r.GetSuccessful(),
			})
		}
	}
	return &pb.Receipt{Proced: true}, nil
}

func (s *ProbeHandler) RequestTask(h *pb.Host, stream pb.ProbeService_RequestTaskServer) error {
	var clientID uint64
	var err error
	if clientID, err = s.Auth.Check(stream.Context()); err != nil {
		return err
	}
	closeCh := make(chan error)
	dao.ServerLock.RLock()
	dao.ServerList[clientID].TaskStream = stream
	dao.ServerList[clientID].TaskClose = closeCh
	dao.ServerLock.RUnlock()
	return <-closeCh
}

func (s *ProbeHandler) ReportSystemState(c context.Context, r *pb.State) (*pb.Receipt, error) {
	var clientID uint64
	var err error
	if clientID, err = s.Auth.Check(c); err != nil {
		return nil, err
	}
	state := model.PB2State(r)
	dao.ServerLock.RLock()
	defer dao.ServerLock.RUnlock()
	dao.ServerList[clientID].LastActive = time.Now()
	dao.ServerList[clientID].State = &state

	// 如果从未记录过，先打点，等到小时时间点时入库
	if dao.ServerList[clientID].PrevHourlyTransferIn == 0 || dao.ServerList[clientID].PrevHourlyTransferOut == 0 {
		dao.ServerList[clientID].PrevHourlyTransferIn = int64(state.NetInTransfer)
		dao.ServerList[clientID].PrevHourlyTransferOut = int64(state.NetOutTransfer)
	}

	return &pb.Receipt{Proced: true}, nil
}

func (s *ProbeHandler) ReportSystemInfo(c context.Context, r *pb.Host) (*pb.Receipt, error) {
	var clientID uint64
	var err error
	if clientID, err = s.Auth.Check(c); err != nil {
		return nil, err
	}
	host := model.PB2Host(r)
	dao.ServerLock.RLock()
	defer dao.ServerLock.RUnlock()
	if dao.Conf.EnableIPChangeNotification &&
		((dao.Conf.Cover == model.ConfigCoverAll && !dao.Conf.IgnoredIPNotificationServerIDs[clientID]) ||
			(dao.Conf.Cover == model.ConfigCoverIgnoreAll && dao.Conf.IgnoredIPNotificationServerIDs[clientID])) &&
		dao.ServerList[clientID].Host != nil &&
		dao.ServerList[clientID].Host.IP != "" &&
		host.IP != "" &&
		dao.ServerList[clientID].Host.IP != host.IP {
		dao.SendNotification(fmt.Sprintf(
			"#探针通知" + "\n" + "IP 变更：" + "\n" + "服务器：%s ，旧IP：%s，新IP：%s。",
			dao.ServerList[clientID].Name, utils.IPDesensitize(dao.ServerList[clientID].Host.IP), utils.IPDesensitize(host.IP)), true)
	}

	// 判断是否是机器重启，如果是机器重启要录入最后记录的流量里面
	if dao.ServerList[clientID].Host.BootTime < host.BootTime {
		dao.ServerList[clientID].PrevHourlyTransferIn = dao.ServerList[clientID].PrevHourlyTransferIn - int64(dao.ServerList[clientID].State.NetInTransfer)
		dao.ServerList[clientID].PrevHourlyTransferOut = dao.ServerList[clientID].PrevHourlyTransferOut - int64(dao.ServerList[clientID].State.NetOutTransfer)
	}

	dao.ServerList[clientID].Host = &host
	return &pb.Receipt{Proced: true}, nil
}