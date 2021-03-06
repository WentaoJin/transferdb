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
package main

import (
	"fmt"

	"github.com/wentaojin/transferdb/pkg/config"

	"github.com/wentaojin/transferdb/db"
)

func main() {
	mysqlCfg := config.TargetConfig{
		Username:      "marvin",
		Password:      "marvin",
		Host:          "192.168.2.30",
		Port:          5000,
		ConnectParams: "charset=utf8mb4&parseTime=True&loc=Local&multiStatements=true&tidb_txn_mode='optimistic'",
		MetaSchema:    "steven",
	}
	engine, err := db.NewMySQLEngineGeneralDB(mysqlCfg, 300)
	if err != nil {
		fmt.Println(err)
	}
	cols, res, err := db.Query(engine.MysqlDB, "select * from t1")
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(cols)
	fmt.Println(res)
}
