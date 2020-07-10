package main

import (
	"html/template"
	"net/http"
	"os"
	"radar-log-parser/go-app/report"
	"radar-log-parser/go-app/settings"
	"radar-log-parser/go-app/utilities"
	"strings"
)

var (
	analysis_details report.AnalysisDetails = report.AnalysisDetails{}
	GroupedIssues                           = make(map[string]report.GroupedStruct)
	NonGroupedIssues                        = make(map[string]map[string]bool)
)

var (
	homeTempl             = template.Must(template.ParseFiles("templates/home.html"))
	upload_configTempl    = template.Must(template.ParseFiles("templates/upload_config_home.html"))
	edit_config_homeTempl = template.Must(template.ParseFiles("templates/editConfigHome.html"))
	edit_configTempl      = template.Must(template.ParseFiles("templates/editConfig.html"))
	delete_configTempl    = template.Must(template.ParseFiles("templates/deleteConfig.html"))
	feedbackTempl         = template.Must(template.ParseFiles("templates/feedback.html"))
	reportTempl           = template.Must(template.ParseFiles("templates/report.html"))
)
var (
	project_id           string   = "log-parser-278319"
	region_id            string   = "ue"
	app_specific_buckets []string = []string{"log-parser-278319.appspot.com", "staging.log-parser-278319.appspot.com", "us.artifacts.log-parser-278319.appspot.com"}
) //TODO: Put in a config file later
var cloudConfigs map[string][]string = make(map[string][]string)
var (
	cfg_edit    string
	bucket_edit string
)

func main() {
	port := os.Getenv("PORT")
	mux := http.NewServeMux()
	fs := http.FileServer(http.Dir("assets"))
	mux.Handle("/assets/", http.StripPrefix("/assets/", fs))
	mux.HandleFunc("/", homeHandler)

	fillConfigMap()

	http.ListenAndServe(":"+port, mux)
}
func fillConfigMap() {

	buckets, err := utilities.GetBuckets(project_id)
	if err != nil {
		return
	}
	for _, bucket := range buckets {
		allow := true
		for _, buckt := range app_specific_buckets {
			if buckt == bucket {
				allow = false
				break
			}
		}
		if allow {
			cfg, err := utilities.GetConfigFiles(bucket)
			if err != nil {
				return
			}
			cloudConfigs[bucket] = cfg
		}

	}
}
func homeHandler(w http.ResponseWriter, r *http.Request) {
	page := r.URL.Path[len("/"):]
	if r.Method != http.MethodPost {
		if len(page) > 5 {
			if strings.Contains(page, "UploadConfig") {
				bucketList := make([]string, 0, len(cloudConfigs))
				for k := range cloudConfigs {
					bucketList = append(bucketList, k)
				}
				upload_configTempl.Execute(w, bucketList)
			} else if strings.Contains(page, "analyzeLog") {
				homeTempl.Execute(w, cloudConfigs)
			} else if strings.Contains(page, "editConfig") {
				edit_config_homeTempl.Execute(w, cloudConfigs)
			} else if strings.Contains(page, "deleteConfig") {
				delete_configTempl.Execute(w, cloudConfigs)
			} else {
				report.LogReport(w, r, analysis_details, GroupedIssues, NonGroupedIssues)
			}
		} else {
			homeTempl.Execute(w, cloudConfigs)
		}
		return
	}
	switch page {
	case "UploadConfig":
		configs, err := settings.UploadConfigFile(r, project_id, cloudConfigs)
		if err != nil {
			return
		}
		cloudConfigs = configs

		feedbackTempl.Execute(w, nil)
	case "editConfig":
		if r.FormValue("action") == "Save" {
			err := settings.SaveConfig(r, bucket_edit, cfg_edit)
			if err != nil {
				return
			}
			feedbackTempl.Execute(w, nil)
		} else {
			bck, cfg, content, err := settings.DisplayConfig(w, r, project_id, region_id)
			bucket_edit = bck
			cfg_edit = cfg
			if err != nil {
				return
			}
			edit_configTempl.Execute(w, content)
		}

	case "deleteConfig":
		configs, err := settings.DeleteConfig(r, project_id, region_id, cloudConfigs)
		if err != nil {
			return
		}
		cloudConfigs = configs
		feedbackTempl.Execute(w, nil)
	default:
		details, grouped, non_grouped, err := report.AnalyseLog(w, r, project_id, region_id)
		if err != nil {
			return
		}
		analysis_details = details
		GroupedIssues = grouped
		NonGroupedIssues = non_grouped
		reportTempl.Execute(w, analysis_details)
	}

}
