/*
 *
 * Copyright 2021 The Vitess Authors.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 * /
 */

package server

import (
	"errors"
	"github.com/dustin/go-humanize"
	"github.com/gin-gonic/gin"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/vitessio/arewefastyet/go/storage/influxdb"
	"github.com/vitessio/arewefastyet/go/storage/mysql"
	"html/template"
)

const (
	ErrorIncorrectConfiguration = "incorrect configuration"

	flagPort                     = "web-port"
	flagTemplatePath             = "web-template-path"
	flagStaticPath               = "web-static-path"
	flagAPIKey                   = "web-api-key"
	flagMode                     = "web-mode"
	flagMicroBenchConfigFile     = "web-microbench-config"
	flagMacroBenchConfigFileOLTP = "web-macrobench-oltp-config"
	flagMacroBenchConfigFileTPCC = "web-macrobench-tpcc-config"
	flagCronSchedule             = "web-cron-schedule"
)

type Server struct {
	port         string
	templatePath string
	staticPath   string
	apiKey       string
	router       *gin.Engine

	dbCfg    *mysql.ConfigDB
	dbClient *mysql.Client

	executionMetricsDBConfig *influxdb.Config
	executionMetricsDBClient *influxdb.Client

	cronSchedule             string
	microbenchConfigPath     string
	macrobenchConfigPathOLTP string
	macrobenchConfigPathTPCC string

	// Mode used to run the server.
	Mode
}

func (s *Server) AddToCommand(cmd *cobra.Command) {
	cmd.Flags().StringVar(&s.port, flagPort, "8080", "Port used for the HTTP server")
	cmd.Flags().StringVar(&s.templatePath, flagTemplatePath, "", "Path to the template directory")
	cmd.Flags().StringVar(&s.staticPath, flagStaticPath, "", "Path to the static directory")
	cmd.Flags().StringVar(&s.apiKey, flagAPIKey, "", "API key used to authenticate requests")
	cmd.Flags().Var(&s.Mode, flagMode, "Specify the mode on which the server will run")

	// execution configuration flags
	cmd.Flags().StringVar(&s.microbenchConfigPath, flagMicroBenchConfigFile, "", "Path to the configuration file used to execute microbenchmark.")
	cmd.Flags().StringVar(&s.macrobenchConfigPathOLTP, flagMacroBenchConfigFileOLTP, "", "Path to the configuration file used to execute OLTP macrobenchmark.")
	cmd.Flags().StringVar(&s.macrobenchConfigPathTPCC, flagMacroBenchConfigFileTPCC, "", "Path to the configuration file used to execute TPCC macrobenchmark.")
	cmd.Flags().StringVar(&s.cronSchedule, flagCronSchedule, "@midnight", "Execution CRON schedule, defaults to everyday at midnight.")
	_ = cmd.MarkFlagRequired(flagMicroBenchConfigFile)
	_ = cmd.MarkFlagRequired(flagMacroBenchConfigFileOLTP)
	_ = cmd.MarkFlagRequired(flagMacroBenchConfigFileTPCC)

	_ = viper.BindPFlag(flagPort, cmd.Flags().Lookup(flagPort))
	_ = viper.BindPFlag(flagTemplatePath, cmd.Flags().Lookup(flagTemplatePath))
	_ = viper.BindPFlag(flagStaticPath, cmd.Flags().Lookup(flagStaticPath))
	_ = viper.BindPFlag(flagAPIKey, cmd.Flags().Lookup(flagAPIKey))
	_ = viper.BindPFlag(flagMode, cmd.Flags().Lookup(flagMode))
	_ = viper.BindPFlag(flagMicroBenchConfigFile, cmd.Flags().Lookup(flagMicroBenchConfigFile))
	_ = viper.BindPFlag(flagMacroBenchConfigFileOLTP, cmd.Flags().Lookup(flagMacroBenchConfigFileOLTP))
	_ = viper.BindPFlag(flagMacroBenchConfigFileTPCC, cmd.Flags().Lookup(flagMacroBenchConfigFileTPCC))
	_ = viper.BindPFlag(flagCronSchedule, cmd.Flags().Lookup(flagCronSchedule))

	if s.dbCfg == nil {
		s.dbCfg = &mysql.ConfigDB{}
	}
	s.dbCfg.AddToCommand(cmd)

	if s.executionMetricsDBConfig == nil {
		s.executionMetricsDBConfig = &influxdb.Config{}
	}
	s.executionMetricsDBConfig.AddToCommand(cmd)
}

func (s Server) isReady() bool {
	return s.port != "" && s.templatePath != "" && s.staticPath != "" && s.apiKey != "" &&
		s.microbenchConfigPath != "" && s.macrobenchConfigPathOLTP != "" && s.macrobenchConfigPathTPCC != ""
}

func (s *Server) Run() error {
	if s.Mode != "" && !s.Mode.correct() {
		return errors.New(ErrorIncorrectMode)
	} else if s.Mode == "" {
		s.Mode.useDefault()
	}

	if slog == nil {
		err := s.initLogger()
		if err != nil {
			return err
		}
		defer cleanLogger()
	}

	if !s.isReady() {
		return errors.New(ErrorIncorrectConfiguration)
	}

	if err := s.createStorages(); err != nil {
		return err
	}

	err := s.createNewCron()
	if err != nil {
		return err
	}

	s.router = gin.Default()
	s.router.SetFuncMap(template.FuncMap{
		"formatFloat": func(f float64) string { return humanize.FormatFloat("#,###.##", f) },
	})

	s.router.Static("/static", s.staticPath)

	s.router.LoadHTMLGlob(s.templatePath + "/*")

	// Information page
	s.router.GET("/information", s.informationHandler)

	// Home page
	s.router.GET("/", s.homeHandler)

	// Compare page
	s.router.GET("/compare", s.compareHandler)

	// Search page
	s.router.GET("/search", s.searchHandler)

	// Request benchmark page
	s.router.GET("/request_benchmark", s.requestBenchmarkHandler)

	// Request benchmark page
	s.router.GET("/microbench", s.microbenchmarkResultsHandler)

	s.router.GET("/microbench/:name", s.microbenchmarkSingleResultsHandler)

	return s.router.Run(":" + s.port)
}

func Run(port, templatePath, staticPath, apiKey string) error {
	s := Server{
		port:         port,
		templatePath: templatePath,
		staticPath:   staticPath,
		apiKey:       apiKey,
	}
	return s.Run()
}
