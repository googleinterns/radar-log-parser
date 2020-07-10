package report

import (
	"html/template"
	"log"
	"net/http"
	"strings"
)

func LogReport(w http.ResponseWriter, r *http.Request, analysisDetails AnalysisDetails, GroupedIssues map[string]GroupedStruct, NonGroupedIssues map[string]map[string]bool) {
	switch file := r.URL.Path[len("/report/"):]; file {
	case analysis_details.FileName:
		template.Must(template.New("details.html").Funcs(template.FuncMap{"add": func(x, y int) int {
			return 0
		}, "addLine": func(x, y string) string {
			return x + "\n" + y
		}, "hightlightIssue": func(line string) bool {
			return true
		}, "detailType": func() string { return "Log" }}).ParseFiles("templates/details.html")).Execute(w, analysis_details.RawLog)
	default:
		if file[:7] == "Details" {
			issue_name := r.URL.Path[len("/report/Details/"):]
			details, ok := GroupedIssues[issue_name]
			if ok {
				log.Println("grouped")
				template.Must(template.New("details.html").Funcs(template.FuncMap{"add": func(x, y int) int {
					return 0
				}, "addLine": func(x, y string) string {
					return x + "\n" + y
				}, "hightlightIssue": func(line string) bool {

					return true
				}, "detailType": func() string { return "Group" }}).ParseFiles("templates/details.html")).Execute(w, details)
			} else {
				log.Println("non_grouped")
				template.Must(template.New("details.html").Funcs(template.FuncMap{"add": func(x, y int) int {
					return x + y
				}, "addLine": func(x, y string) string {
					return x + "\n" + y
				}, "hightlightIssue": func(line string) bool {
					_, ok := NonGroupedIssues[issue_name][line]
					return ok
				}, "detailType": func() string { return "nonGroup" }}).ParseFiles("templates/details.html")).Execute(w, strings.Split(analysis_details.RawLog, "\n"))
			}
		} else {
			template.Must(template.New("details.html").Funcs(template.FuncMap{"add": func(x, y int) int {
				return 0
			}, "addLine": func(x, y string) string {
				return x + "\n" + y
			}, "hightlightIssue": func(line string) bool {

				return true
			}, "detailType": func() string { return "Log" }}).ParseFiles("templates/details.html")).Execute(w, analysis_details.SpecificProcess[file])

		}
	}

}
