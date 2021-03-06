/*
Copyright © 2020 Marvin

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package taskflow

import (
	"fmt"
	"strings"
	"time"

	"github.com/wentaojin/transferdb/pkg/config"

	"github.com/xxjwxc/gowp/workpool"

	"github.com/wentaojin/transferdb/zlog"
	"go.uber.org/zap"

	"github.com/wentaojin/transferdb/db"
)

// 表数据应用 -> 全量任务
func applierTableFullRecord(targetSchemaName, targetTableName string, workerThreads int, sqlSlice []string, engine *db.Engine) error {
	startTime := time.Now()
	zlog.Logger.Info("single full table data applier start",
		zap.String("schema", targetSchemaName),
		zap.String("table", targetTableName))

	wp := workpool.New(workerThreads)
	for _, sql := range sqlSlice {
		s := sql
		wp.DoWait(func() error {
			_, err := engine.MysqlDB.Exec(s)
			if err != nil {
				return fmt.Errorf("single full table data bulk insert mysql [%s] falied:%v", sql, err)
			}
			return nil
		})
	}
	if err := wp.Wait(); err != nil {
		return err
	}
	if !wp.IsDone() {
		return fmt.Errorf("single full table data applier meet error")
	}
	endTime := time.Now()
	zlog.Logger.Info("single full table data applier finished",
		zap.String("schema", targetSchemaName),
		zap.String("table", targetTableName),
		zap.String("cost", endTime.Sub(startTime).String()))
	return nil
}

// 表数据应用 -> 增量任务
func applierTableIncrementRecord(p *IncrementPayload) error {
	sql := strings.Join(p.MySQLRedo, ";")
	//zlog.Logger.Info("increment applier sql", zap.String("sql", sql))
	_, err := p.Engine.MysqlDB.Exec(sql)
	if err != nil {
		return fmt.Errorf("single increment table data insert mysql [%s] falied:%v", sql, err)
	}
	// 数据写入完毕，更新元数据 checkpoint 表
	// 如果同步中断，数据同步使用会以 global_scn 为准，也就是会进行重复消费
	if err := p.Engine.UpdateTableIncrementMetaALLSCNRecord(p.SourceSchema, p.SourceTable, p.OperationType, p.GlobalSCN, p.SourceTableSCN); err != nil {
		zlog.Logger.Error("update table increment scn record failed",
			zap.String("payload", p.Marshal()),
			zap.Error(err))
		return err
	}
	return nil
}

func applyOracleRedoIncrementRecord(cfg *config.CfgFile, engine *db.Engine, logminerContentMap map[string][]db.LogminerContent) error {
	// 应用当前日志文件中所有记录
	wp := workpool.New(cfg.AllConfig.ApplyThreads)
	for tableName, lcs := range logminerContentMap {
		rowsResult := lcs
		tbl := tableName
		wp.DoWait(func() error {
			if len(rowsResult) > 0 {
				var (
					done        = make(chan bool)
					taskQueue   = make(chan IncrementPayload, cfg.AllConfig.WorkerQueue)
					resultQueue = make(chan IncrementResult, cfg.AllConfig.WorkerQueue)
				)
				// 获取增量执行结果
				go GetIncrementResult(done, resultQueue)

				// 转换捕获内容以及数据应用
				go func(engine *db.Engine, tbl, targetSchemaName string, rowsResult []db.LogminerContent, taskQueue chan IncrementPayload) {
					defer func() {
						if err := recover(); err != nil {
							zlog.Logger.Fatal("translatorAndApplyOracleIncrementRecord",
								zap.String("schema", cfg.TargetConfig.SchemaName),
								zap.String("table", tbl),
								zap.Error(fmt.Errorf("%v", err)))
						}
					}()
					if err := translatorAndApplyOracleIncrementRecord(
						engine,
						tbl,
						targetSchemaName,
						rowsResult, taskQueue); err != nil {
						return
					}
				}(engine, tbl, cfg.TargetConfig.SchemaName, rowsResult, taskQueue)

				// 必须在任务分配和获取结果后创建工作池
				go CreateWorkerPool(cfg.AllConfig.WorkerThreads, taskQueue, resultQueue)
				// 等待执行完成
				<-done

				return nil
			}
			zlog.Logger.Warn("increment table log file logminer null data, transferdb will continue to capture",
				zap.String("mysql schema", cfg.TargetConfig.SchemaName),
				zap.String("table", tbl),
				zap.String("status", "success"))
			return nil
		})
	}
	if err := wp.Wait(); err != nil {
		return err
	}
	if !wp.IsDone() {
		return fmt.Errorf("logminerContentMap concurrency meet error")
	}
	return nil
}
