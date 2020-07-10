package report

import (
	"html/template"
	"net/http"
	"strconv"
	"strings"
)

func LogReport(w http.ResponseWriter, r *http.Request, analysisDetails AnalysisDetails, GroupedIssues map[string]GroupedStruct, NonGroupedIssues map[string]map[string]bool) {
	switch file := r.URL.Path[len("/report/"):]; file {
	case analysis_details.FileName:
		template.Must(template.New("details.html").Funcs(template.FuncMap{"detailType": func() string { return "Log" }}).ParseFiles("templates/details.html")).Execute(w, analysis_details.RawLog)

	default:
		if file[:7] == "Details" {
			issue_name := r.URL.Path[len("/report/Details/"):]
			details, ok := GroupedIssues[issue_name]
			if ok {
				template.Must(template.New("details.html").Funcs(template.FuncMap{"detailType": func() string { return "Group" }}).ParseFiles("templates/details.html")).Execute(w, details)
			} else {

				issue_num, _ := strconv.Atoi(analysis_details.Issues[issue_name]["Number"])
				details, hightlight := nonGroupDetails(analysis_details.RawLog, NonGroupedIssues[issue_name], issue_num)
				detail := struct {
					Highlight map[int]bool
					Details   []string
				}{
					hightlight, details,
				}
				template.Must(template.New("details.html").Funcs(template.FuncMap{"detailType": func() string { return "nonGroup" }}).ParseFiles("templates/details.html")).Execute(w, detail)

			}
		} else {
			template.Must(template.New("details.html").Funcs(template.FuncMap{"detailType": func() string { return "Log" }}).ParseFiles("templates/details.html")).Execute(w, analysis_details.SpecificProcess[file])

		}
	}

}
func nonGroupDetails(fContent string, nonGroupedIssues map[string]bool, num_issue int) ([]string, map[int]bool) {
	details := make([]string, 0, num_issue*2+1)
	hightlight := map[int]bool{ //index=> true = must be highlight
	}
	last_found_index := 0
	contentLines := strings.Split(fContent, "\n")
	found_issue := 0
	for index, line := range contentLines {
		if nonGroupedIssues[line] {
			if last_found_index != index {
				details = append(details, strings.Join(contentLines[last_found_index:index], "\n"))
				hightlight[len(details)-1] = false
			}
			details = append(details, line)
			last_found_index = index + 1
			hightlight[len(details)-1] = true
			found_issue += 1

			if found_issue == num_issue {
				details = append(details, strings.Join(contentLines[last_found_index:], "\n"))
				hightlight[len(details)-1] = false
				break
			}
		}

	}
	return details, hightlight
}
