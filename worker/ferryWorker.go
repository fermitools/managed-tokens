package worker

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"strings"
	"text/template"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"

	"github.com/shreyb/managed-tokens/utils"
)

var ferryURLUIDTemplate = template.Must(template.New("ferry").Parse("{{.URL}}:{{.Port}}/{{.API}}?username={{.Username}}"))

type ferryUIDResponse struct {
	FerryStatus string   `json:"ferry_status"`
	FerryError  []string `json:"ferry_error"`
	FerryOutput struct {
		ExpirationDate string `json:"expirationdate"`
		FullName       string `json:"fullname"`
		GroupAccount   bool   `json:"groupaccount"`
		Status         bool   `json:"status"`
		Uid            int    `json:"uid"`
		VOPersonId     string `json:"vopersonid"`
	} `json:"ferry_output"`
}

func GetFERRYUIDData(httpsClient *http.Client, username string, ferryDataChan chan<- *utils.UIDEntryFromFerry) {
	ferryAPIConfig := struct{ URL, Port, API, Username string }{
		URL:      viper.GetString("ferryURL"),
		Port:     viper.GetString("ferryPort"),
		API:      "getUserInfo",
		Username: username,
	}

	var b strings.Builder
	if err := ferryURLUIDTemplate.Execute(&b, ferryAPIConfig); err != nil {
		log.Errorf("Could not execute ferryURLUID template")
		log.Fatal(err)
	}

	resp, err := httpsClient.Get(b.String())
	if err != nil {
		log.WithField("account", username).Error("Attempt to get UID from FERRY failed")
		log.WithField("account", username).Error(err)
		return
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.WithField("account", username).Error("Could not read body from HTTP response")
	}

	parsedResponse := ferryUIDResponse{}
	if err := json.Unmarshal(body, &parsedResponse); err != nil {
		log.WithField("account", username).Error("Could not unmarshal FERRY response")
		log.WithField("account", username).Error(err)
	}

	entry := utils.UIDEntryFromFerry{
		Username: username,
		Uid:      parsedResponse.FerryOutput.Uid,
	}

	ferryDataChan <- &entry
}