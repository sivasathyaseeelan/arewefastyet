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
	"fmt"
	"net/http"

	"github.com/vitessio/arewefastyet/go/exec"

	"github.com/gin-gonic/gin"
	"github.com/vitessio/arewefastyet/go/exec/metrics"
	"github.com/vitessio/arewefastyet/go/tools/git"
	"github.com/vitessio/arewefastyet/go/tools/macrobench"
	"github.com/vitessio/arewefastyet/go/tools/microbench"
)

func handleRenderErrors(c *gin.Context, err error) {
	if err == nil {
		return
	}
	slog.Error(err.Error())
	c.HTML(http.StatusOK, "error.tmpl", gin.H{
		"title": "Vitess benchmark - Error",
		"url":   c.FullPath(),
	})
}

func (s *Server) dailyHandler(c *gin.Context) {
	planner := getPlannerVersion()

	oltpData, err := macrobench.GetResultsForLastDays("OLTP", "cron", planner, 31, s.dbClient)
	if err != nil {
		slog.Warn(err.Error())
	}

	tpccData, err := macrobench.GetResultsForLastDays("TPCC", "cron", planner, 31, s.dbClient)
	if err != nil {
		slog.Warn(err.Error())
	}

	c.HTML(http.StatusOK, "cron.tmpl", gin.H{
		"title":     "Vitess benchmark - cron",
		"data_oltp": oltpData,
		"data_tpcc": tpccData,
	})
}

func (s *Server) analyticsHandler(c *gin.Context) {
	planner := getPlannerVersion()

	oltpData, err := macrobench.GetResultsForLastDays("OLTP", "cron_analytics", planner, 31, s.dbClient)
	if err != nil {
		slog.Warn(err.Error())
	}

	for i, data := range oltpData {
		m, err := metrics.GetExecutionMetricsSQL(s.dbClient, data.ExecUUID)
		if err != nil {
			slog.Warn(err.Error())
		}
		oltpData[i].Metrics = m
	}

	c.HTML(http.StatusOK, "analytics.tmpl", gin.H{
		"title":     "Vitess benchmark - analytics",
		"data_oltp": oltpData,
	})
}

func getPlannerVersion() macrobench.PlannerVersion {
	return macrobench.Gen4Planner
}

func (s *Server) homeHandler(c *gin.Context) {
	c.HTML(http.StatusOK, "index.tmpl", gin.H{
		"title": "Vitess benchmark",
	})
}

func (s *Server) statusHandler(c *gin.Context) {
	recentExecutions, err := exec.GetRecentExecutions(s.dbClient)
	if err != nil {
		handleRenderErrors(c, err)
		return
	}
	c.HTML(http.StatusOK, "status.tmpl", gin.H{
		"title":      "Vitess benchmark - Status",
		"queue":      queue,
		"executions": recentExecutions,
	})
}

func (s *Server) compareHandler(c *gin.Context) {
	planner := getPlannerVersion()
	right := c.Query("r")
	left := c.Query("c")

	leftSHA := map[string]interface{}{
		"SHA":   left,
		"short": git.ShortenSHA(left),
	}
	rightSHA := map[string]interface{}{
		"SHA":   right,
		"short": git.ShortenSHA(right),
	}
	if right == "" || left == "" {
		c.HTML(http.StatusOK, "compare.tmpl", gin.H{
			"title": "Vitess benchmark",
			"right": rightSHA,
			"left":  leftSHA,
		})
		return
	}

	// Compare Macrobenchmarks for the two given SHAs.
	macrosMatrices, err := macrobench.CompareMacroBenchmarks(s.dbClient, right, left, planner, s.benchmarkTypes)
	if err != nil {
		handleRenderErrors(c, err)
		return
	}

	// Compare Microbenchmarks for the two given SHAs.
	microsMatrix, err := microbench.Compare(s.dbClient, right, left)
	if err != nil {
		handleRenderErrors(c, err)
		return
	}

	c.HTML(http.StatusOK, "compare.tmpl", gin.H{
		"title":          "Vitess benchmark",
		"right":          rightSHA,
		"left":           leftSHA,
		"microbenchmark": microsMatrix,
		"macrobenchmark": macrosMatrices,
	})
}

func (s *Server) searchHandler(c *gin.Context) {
	planner := getPlannerVersion()
	search := c.Query("s")
	if search == "" {
		c.HTML(http.StatusOK, "search.tmpl", gin.H{
			"title": "Vitess benchmark",
		})
		return
	}

	macros, err := macrobench.GetDetailsArraysFromAllTypes(search, planner, s.dbClient, s.benchmarkTypes)
	if err != nil {
		handleRenderErrors(c, err)
		return
	}

	micro, err := microbench.GetResultsForGitRef(search, s.dbClient)
	if err != nil {
		handleRenderErrors(c, err)
		return
	}
	micro = micro.ReduceSimpleMedianByName()

	c.HTML(http.StatusOK, "search.tmpl", gin.H{
		"title":          "Vitess benchmark",
		"search":         search,
		"shortSHA":       git.ShortenSHA(search),
		"microbenchmark": micro,
		"macrobenchmark": macros,
	})
}

func (s *Server) requestBenchmarkHandler(c *gin.Context) {
	c.HTML(http.StatusOK, "request_benchmark.tmpl", gin.H{
		"title": "Vitess benchmark",
	})
}

func (s *Server) microbenchmarkResultsHandler(c *gin.Context) {
	var err error

	// get all the latest releases and the last daily job for main
	allReleases, err := git.GetLatestVitessReleaseCommitHash(s.getVitessPath())
	if err != nil {
		handleRenderErrors(c, err)
		return
	}
	lastrunDailySHA, err := exec.GetLatestDailyJobForMicrobenchmarks(s.dbClient)
	if err != nil {
		handleRenderErrors(c, err)
		return
	}
	mainRelease := []*git.Release{{
		Name:       "main",
		CommitHash: lastrunDailySHA,
	}}
	allReleases = append(mainRelease, allReleases...)
	// get all the latest release branches as well
	allReleaseBranches, err := git.GetLatestVitessReleaseBranchCommitHash(s.getVitessPath())
	if err != nil {
		handleRenderErrors(c, err)
		return
	}
	allReleases = append(allReleases, allReleaseBranches...)

	// initialize left tag and the corresponding sha
	leftTag := c.Query("ltag")
	leftSHA := ""
	if leftTag == "" {
		// get the last release sha if rightSHA is not specified
		rel, err := git.GetLastReleaseAndCommitHash(s.getVitessPath())
		if err != nil {
			handleRenderErrors(c, err)
			return
		}
		leftSHA = rel.CommitHash
		leftTag = rel.Name
	} else {
		leftSHA, err = findSHA(allReleases, leftTag)
		if err != nil {
			handleRenderErrors(c, err)
			return
		}
	}

	// initialize right tag and the corresponding sha
	rightTag := c.Query("rtag")
	rightSHA := ""
	if rightTag == "" {
		// get the latest daily job if leftTag is not specified
		rightTag = "main"
		rightSHA = lastrunDailySHA
	} else {
		rightSHA, err = findSHA(allReleases, rightTag)
		if err != nil {
			handleRenderErrors(c, err)
			return
		}
	}

	// Get the results from the SHAs
	leftMbd, err := microbench.GetResultsForGitRef(leftSHA, s.dbClient)
	if err != nil {
		handleRenderErrors(c, err)
		return
	}
	leftMbd = leftMbd.ReduceSimpleMedianByName()
	rightMbd, err := microbench.GetResultsForGitRef(rightSHA, s.dbClient)
	if err != nil {
		handleRenderErrors(c, err)
		return
	}
	rightMbd = rightMbd.ReduceSimpleMedianByName()

	matrix := microbench.MergeDetails(rightMbd, leftMbd)
	c.HTML(http.StatusOK, "microbench.tmpl", gin.H{
		"title":        "Vitess benchmark - microbenchmark",
		"leftSHA":      leftSHA,
		"rightSHA":     rightSHA,
		"leftTag":      leftTag,
		"rightTag":     rightTag,
		"allReleases":  allReleases,
		"resultMatrix": matrix,
	})
}

func findSHA(releases []*git.Release, tag string) (string, error) {
	for _, release := range releases {
		if release.Name == tag {
			return release.CommitHash, nil
		}
	}
	return "", fmt.Errorf("unknown tag provided %s", tag)
}

func (s *Server) microbenchmarkSingleResultsHandler(c *gin.Context) {
	name := c.Param("name")
	subBenchmarkName := c.Query("subBenchmarkName")

	results, err := microbench.GetLatestResultsFor(name, subBenchmarkName, 10, s.dbClient)
	if err != nil {
		handleRenderErrors(c, err)
		return
	}
	results = results.ReduceSimpleMedianByGitRef()
	results.SortByDate()
	c.HTML(http.StatusOK, "microbench_single.tmpl", gin.H{
		"title":            "Vitess benchmark - microbenchmark - " + name,
		"name":             name,
		"subBenchmarkName": subBenchmarkName,
		"results":          results,
	})
}

func (s *Server) macrobenchmarkResultsHandler(c *gin.Context) {
	var err error
	planner := getPlannerVersion()
	// get all the latest releases and the last daily job for main
	allReleases, err := git.GetLatestVitessReleaseCommitHash(s.getVitessPath())
	if err != nil {
		handleRenderErrors(c, err)
		return
	}
	lastrunDailySHA, err := exec.GetLatestDailyJobForMacrobenchmarks(s.dbClient)
	if err != nil {
		handleRenderErrors(c, err)
		return
	}
	mainRelease := []*git.Release{{
		Name:       "main",
		CommitHash: lastrunDailySHA,
	}}
	allReleases = append(mainRelease, allReleases...)
	// get all the latest release branches as well
	allReleaseBranches, err := git.GetLatestVitessReleaseBranchCommitHash(s.getVitessPath())
	if err != nil {
		handleRenderErrors(c, err)
		return
	}
	allReleases = append(allReleases, allReleaseBranches...)

	// initialize left tag and the corresponding sha
	leftTag := c.Query("ltag")
	leftSHA := ""
	if leftTag == "" {
		// get the last release sha if rightSHA is not specified
		rel, err := git.GetLastReleaseAndCommitHash(s.getVitessPath())
		if err != nil {
			handleRenderErrors(c, err)
			return
		}
		leftSHA = rel.CommitHash
		leftTag = rel.Name
	} else {
		leftSHA, err = findSHA(allReleases, leftTag)
		if err != nil {
			handleRenderErrors(c, err)
			return
		}
	}

	// initialize right tag and the corresponding sha
	rightTag := c.Query("rtag")
	rightSHA := ""
	if rightTag == "" {
		// get the latest daily job if leftTag is not specified
		rightTag = "main"
		rightSHA = lastrunDailySHA
	} else {
		rightSHA, err = findSHA(allReleases, rightTag)
		if err != nil {
			handleRenderErrors(c, err)
			return
		}
	}

	// Compare Macrobenchmarks for the two given SHAs.
	macrosMatrices, err := macrobench.CompareMacroBenchmarks(s.dbClient, rightSHA, leftSHA, planner, s.benchmarkTypes)
	if err != nil {
		handleRenderErrors(c, err)
		return
	}
	c.HTML(http.StatusOK, "macrobench.tmpl", gin.H{
		"title":          "Vitess benchmark - macrobenchmark",
		"leftSHA":        leftSHA,
		"rightSHA":       rightSHA,
		"leftTag":        leftTag,
		"rightTag":       rightTag,
		"allReleases":    allReleases,
		"macrobenchmark": macrosMatrices,
	})
}

func (s *Server) macrobenchmarkQueriesDetails(c *gin.Context) {
	planner := getPlannerVersion()
	gitRef := c.Param("git_ref")
	macroType := macrobench.Type(c.Query("type"))

	if gitRef == "" || macroType == "" {
		c.HTML(http.StatusOK, "error.tmpl", gin.H{
			"title": "Vitess benchmark - macrobenchmark queries",
			"error": "Invalid git SHA or macrobenchmark type (i.e: oltp, tpcc).",
		})
		return
	}
	plans, err := macrobench.GetVTGateSelectQueryPlansWithFilter(gitRef, macroType, planner, s.dbClient)
	if err != nil {
		handleRenderErrors(c, err)
		return
	}
	c.HTML(http.StatusOK, "macrobench_queries.tmpl", gin.H{
		"title": "Vitess benchmark - macrobenchmark queries",
		"gitRef": map[string]interface{}{
			"SHA":   gitRef,
			"short": git.ShortenSHA(gitRef),
		},
		"macroType": macroType.String(),
		"planner":   string(planner),
		"plans":     plans,
	})
}

func (s *Server) macrobenchmarkCompareQueriesDetails(c *gin.Context) {
	leftGitRef := c.Query("left")
	rightGitRef := c.Query("right")
	macroType := macrobench.Type(c.Query("type"))

	if leftGitRef == "" || rightGitRef == "" || macroType == "" {
		c.HTML(http.StatusOK, "error.tmpl", gin.H{
			"title": "Vitess benchmark - compare macrobenchmark queries",
			"error": "Invalid git SHAs or macrobenchmark type (i.e: oltp, tpcc).",
		})
		return
	}

	planner := getPlannerVersion()
	leftPlanner := macrobench.PlannerVersion(c.Query("left_planner"))
	rightPlanner := macrobench.PlannerVersion(c.Query("right_planner"))

	if leftPlanner == "" {
		leftPlanner = planner
	}
	if rightPlanner == "" {
		rightPlanner = planner
	}

	plansLeft, err := macrobench.GetVTGateSelectQueryPlansWithFilter(leftGitRef, macroType, leftPlanner, s.dbClient)
	if err != nil {
		handleRenderErrors(c, err)
		return
	}
	plansRight, err := macrobench.GetVTGateSelectQueryPlansWithFilter(rightGitRef, macroType, rightPlanner, s.dbClient)
	if err != nil {
		handleRenderErrors(c, err)
		return
	}
	comparison := macrobench.CompareVTGateQueryPlans(plansLeft, plansRight)
	c.HTML(http.StatusOK, "macrobench_compare_queries.tmpl", gin.H{
		"title": "Vitess benchmark - macrobenchmark queries",
		"gitRefLeft": map[string]interface{}{
			"SHA":   leftGitRef,
			"short": git.ShortenSHA(leftGitRef),
		},
		"gitRefRight": map[string]interface{}{
			"SHA":   rightGitRef,
			"short": git.ShortenSHA(rightGitRef),
		},
		"macroType":       macroType.String(),
		"plannerLeft":     string(leftPlanner),
		"plannerRight":    string(rightPlanner),
		"plansComparison": comparison,
	})
}

func (s *Server) v3VsGen4Handler(c *gin.Context) {
	var err error

	// get all the latest releases and the last daily job for main
	allReleases, err := git.GetLatestVitessReleaseCommitHash(s.getVitessPath())
	if err != nil {
		handleRenderErrors(c, err)
		return
	}
	lastrunDailySHA, err := exec.GetLatestDailyJobForMacrobenchmarks(s.dbClient)
	if err != nil {
		handleRenderErrors(c, err)
		return
	}
	mainRelease := []*git.Release{{
		Name:       "main",
		CommitHash: lastrunDailySHA,
	}}
	allReleases = append(mainRelease, allReleases...)
	// get all the latest release branches as well
	allReleaseBranches, err := git.GetLatestVitessReleaseBranchCommitHash(s.getVitessPath())
	if err != nil {
		handleRenderErrors(c, err)
		return
	}
	allReleases = append(allReleases, allReleaseBranches...)

	// initialize tag and the corresponding sha
	tag := c.Query("tag")
	sha := ""
	if tag == "" {
		// get the latest daily job if tag is not specified
		tag = "main"
		sha = lastrunDailySHA
	} else {
		sha, err = findSHA(allReleases, tag)
		if err != nil {
			handleRenderErrors(c, err)
			return
		}
	}

	// Compare Macrobenchmarks for the two planners for the given SHA.
	macrosMatrices, err := macrobench.ComparePlanners(s.dbClient, sha, s.benchmarkTypes)
	if err != nil {
		handleRenderErrors(c, err)
		return
	}
	c.HTML(http.StatusOK, "v3VsGen4.tmpl", gin.H{
		"title":          "Vitess v3 vs Gen4 Planner",
		"sha":            sha,
		"tag":            tag,
		"allReleases":    allReleases,
		"macrobenchmark": macrosMatrices,
	})
}
