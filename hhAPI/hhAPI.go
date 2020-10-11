package hhAPI

// пакет для работы с API hh.ru

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/semyon-dev/hh.ru-search/elastic"
	"io"
	"log"
	"net/http"
	"os"
	"sync"
)

// тип запроса
type reqType uint8

// макс кол-во стека неудачных запросов после которого нужно запрашивать повторно
const maxFailedReqs = 10

// тип - страница
const pageReq reqType = 1

// тип - полная вакансия по ID
const fullVacancyReq reqType = 2

// неуспешные запросы
var FailedRequests struct {
	reqs map[string]reqType
	sync.Mutex
}

// получение вакансий за 1 страницу
func GetByText(text string, page string) (vacanciesPerPage *elastic.VacanciesPerPage) {

	if page == "" {
		page = "0"
	}

	client := &http.Client{}
	req, err := http.NewRequest("GET", "https://api.hh.ru/vacancies", nil)

	if err != nil {
		log.Println(err)
		return
	}

	q := req.URL.Query()
	q.Add("text", text)
	q.Add("per_page", "25")
	q.Add("page", page)
	req.URL.RawQuery = q.Encode()
	req.Header.Add("User-Agent", "Localhost 1.0") // добавляем заголовок User-Agent
	resp, err := client.Do(req)
	if err != nil {
		log.Println(err)
		return
	}
	defer func() {
		err = resp.Body.Close()
		if err != nil {
			log.Println(err)
		}
	}()

	if resp.StatusCode != 200 {
		addFailedRequest(pageReq, req.URL.String())
		fmt.Println(resp.Status)
	}

	buf := new(bytes.Buffer)
	_, err = io.Copy(buf, resp.Body)
	if err != nil {
		log.Println(err)
	}
	err = json.NewDecoder(buf).Decode(&vacanciesPerPage)
	if err != nil {
		log.Println(err)
	}
	return vacanciesPerPage
}

// получение вакансии с описанием по ID
func GetByID(id string) (fullVacancy *elastic.FullVacancy, status int) {

	client := &http.Client{}
	req, err := http.NewRequest("GET", "https://api.hh.ru/vacancies/"+id, nil)

	if err != nil {
		log.Println(err)
		return
	}

	req.Header.Add("User-Agent", "Localhost 1.0") // добавляем заголовок User-Agent
	resp, err := client.Do(req)
	if err != nil {
		log.Println(err)
		os.Exit(1)
		return
	}
	defer func() {
		err = resp.Body.Close()
		if err != nil {
			log.Println(err)
		}
	}()

	if resp.StatusCode != 200 {
		addFailedRequest(fullVacancyReq, req.URL.String())
		fmt.Println(resp.Status)
	}

	buf := new(bytes.Buffer) // буфер для чтения и записи
	_, err = io.Copy(buf, resp.Body)
	if err != nil {
		log.Println(err)
	}
	err = json.NewDecoder(buf).Decode(&fullVacancy)
	if err != nil {
		log.Println(err)
	}
	return fullVacancy, resp.StatusCode
}

// добавление неудачного запроса
func addFailedRequest(reqT reqType, URL string) {

	go func() {
		FailedRequests.Lock()
		if FailedRequests.reqs == nil {
			FailedRequests.reqs = make(map[string]reqType)
		}
		FailedRequests.reqs[URL] = reqT
		if len(FailedRequests.reqs) > maxFailedReqs {
			fmt.Println("делаем повторные запросы (неудачные)")
			for i, reqType := range FailedRequests.reqs {
				resp, err := http.Get(i)
				if err != nil {
					log.Println(err)
				}
				if resp.StatusCode == 200 {
					delete(FailedRequests.reqs, i)
				}
				buf := new(bytes.Buffer)
				_, err = io.Copy(buf, resp.Body)
				if err != nil {
					log.Println(err)
				}
				switch reqType {
				case pageReq:
					var vacs elastic.VacanciesPerPage
					var fullVacancies = elastic.FullVacancies{}
					err = json.NewDecoder(buf).Decode(&vacs)
					if err != nil {
						log.Println(err)
					}
					for i := 0; i < len(vacs.Items); i++ {
						var fullVac *elastic.FullVacancy
						fullVac, _ = GetByID(vacs.Items[i].Id)
						fullVacancies = append(fullVacancies, fullVac)
					}
					fullVacancies.InsertMany()
				case fullVacancyReq:
					var vacancy *elastic.FullVacancy
					err = json.Unmarshal(buf.Bytes(), &vacancy)
					if err != nil {
						log.Println(err)
					}
					vacancy.InsertOne()
				}
				err = resp.Body.Close()
				if err != nil {
					log.Println(err)
				}
			}
		}
		FailedRequests.Unlock()
	}()
}
