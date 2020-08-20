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

type Feedback struct {
	Error   bool
	Content string
}

var (
	feedBack       = Feedback{}
	fullLogDetails = report.FullDetails{}
	cfg_file       = report.Config{}
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
	mux.HandleFunc("/UploadConfig", uploadCfgHandler)
	mux.HandleFunc("/editConfig", editCfgHandler)
	mux.HandleFunc("/deleteConfig", deleteCfgHandler)
	mux.HandleFunc("/report", reportHandler)
	mux.HandleFunc("/report/", reportHandler)
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
func getFeedBack(err error, content string) {
	if err != nil {
		feedBack.Error = true
		feedBack.Content = err.Error()
	} else {
		feedBack.Error = false
		feedBack.Content = content
	}
}
func homeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		homeTempl.Execute(w, cloudConfigs)
	} else {
		loadLogLevel(w, r, fullLogDetails.Analysis_details.Platform, fullLogDetails.Analysis_details.RawLog)
	}
}
func fillUploadCfgPage(w http.ResponseWriter, r *http.Request) {
	bucketList := make([]string, 0, len(cloudConfigs))
	for k := range cloudConfigs {
		bucketList = append(bucketList, k)
	}
	upload_configTempl.Execute(w, bucketList)
}
func loadLogLevel(w http.ResponseWriter, r *http.Request, platform string, rawlog string) {
	r.ParseMultipartForm(10 << 20)
	level := r.FormValue("selectedLevel")
	logContent := report.GetLogLeveldetails(platform, level, rawlog)
	w.Write([]byte(logContent))
}
func uploadCfgHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		fillUploadCfgPage(w, r)
	} else {
		configs, err := settings.UploadConfigFile(r, project_id, cloudConfigs)
		getFeedBack(err, "Upload Config")
		if err == nil {
			cloudConfigs = configs
		}
		cloudConfigs = configs
		feedbackTempl.Execute(w, feedBack)
	}
}
func editCfgHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		edit_config_homeTempl.Execute(w, cloudConfigs)
	} else {
		if r.FormValue("action") == "Save" {
			err := settings.SaveConfig(r, bucket_edit, cfg_edit)
			getFeedBack(err, "Edit Config")
			feedbackTempl.Execute(w, feedBack)
		} else {
			bck, cfg, content, err := settings.DisplayConfig(w, r, project_id, region_id)
			bucket_edit = bck
			cfg_edit = cfg
			if err != nil {
				getFeedBack(err, "Edit Config")
				feedbackTempl.Execute(w, feedBack)
				return
			}
			edit_configTempl.Execute(w, content)
		}
	}
}
func deleteCfgHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		delete_configTempl.Execute(w, cloudConfigs)
	} else {
		configs, err := settings.DeleteConfig(r, project_id, region_id, cloudConfigs)
		getFeedBack(err, "Delete Config")
		if err == nil {
			cloudConfigs = configs
		}
		feedbackTempl.Execute(w, feedBack)
	}
}

func loadAnalyseLog(w http.ResponseWriter, r *http.Request, fullDetails *report.FullDetails, conf_file *report.Config) {
	err := report.AnalyseLog(w, r, project_id, region_id, fullDetails, conf_file)
	if err != nil {
		getFeedBack(err, "Log Analysis Error")
		feedbackTempl.Execute(w, feedBack)
		return
	}
	reportTempl.Execute(w, fullLogDetails.Analysis_details)
}
func reportHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		report.LogReport(w, r, &fullLogDetails, &cfg_file)
	} else {
		page := r.URL.Path[len("/report"):]
		if strings.Contains(page, "/events/details") {
			report.LoadEventDetails(w, r, fullLogDetails.Analysis_details.RawLog, fullLogDetails.ImportantEvents)
		} else {
			loadAnalyseLog(w, r, &fullLogDetails, &cfg_file)
		}
	}
}
