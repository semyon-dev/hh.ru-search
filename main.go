package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/semyon-dev/hh.ru-search/elastic"
	"github.com/semyon-dev/hh.ru-search/hhAPI"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

var portFlag string // порт

func init() {
	flag.StringVar(&portFlag, "p", "8080", "you can choose specify the port")
}

func main() {

	flag.Parse() // парсинг флагов
	elastic.Init()

	if !elastic.IsIndexExist("full_vacancies") {
		fmt.Println("Парсинг изначальных данных...")
		parseVacancies("Golang")
		fmt.Println("Парсинг изначальных данных закончился")
	}

	mux := http.NewServeMux()
	mux.Handle("/vacancies/", http.HandlerFunc(vacanciesHandler))
	mux.Handle("/parse-vacancies/", http.HandlerFunc(parseVacanciesHandler))

	fmt.Println("Запускаем REST API на порту " + portFlag + " [описание API в README]")

	err := http.ListenAndServe(":"+portFlag, mux)
	if err != nil {
		log.Fatal(err)
	}
}

// парсер вакансий из hh.ru
func parseVacanciesHandler(w http.ResponseWriter, r *http.Request) {

	searchText := r.URL.Query().Get("text")
	if searchText == "" {
		searchText = "Golang"
	}

	parseVacancies(searchText)

	_, err := w.Write([]byte(`{"message":"ok"}`))
	if err != nil {
		log.Println(err)
	}
}

// handler для парсинга вакансий
func parseVacancies(searchText string) {

	// получаем первую страницу вакансий
	vacanciesPerPage := hhAPI.GetByText(searchText, "")

	var wg sync.WaitGroup
	wg.Add(vacanciesPerPage.Pages)

	var m sync.Mutex

	var fullVacancies elastic.FullVacancies

	// далее запрашиваем каждую страницу
	for pageNumber := 0; pageNumber < vacanciesPerPage.Pages; pageNumber++ {
		go func(page int) {
			defer wg.Done()
			vacanciesPerPage := hhAPI.GetByText(searchText, strconv.Itoa(page))
			var status = 200
			for i := 0; i < len(vacanciesPerPage.Items); i++ {
				// если status == 429 то лимит API превышен
				// но мы не теряем вакансию, запрос будет сделан позже из списка неудачных
				if status == 429 {
					time.Sleep(200 * time.Millisecond)
				}
				var fullVac *elastic.FullVacancy
				fullVac, status = hhAPI.GetByID(vacanciesPerPage.Items[i].Id)
				m.Lock()
				fullVacancies = append(fullVacancies, fullVac)
				m.Unlock()
			}
		}(pageNumber)
	}

	wg.Wait()
	fullVacancies.InsertMany()
}

// handler для получения вакансий
func vacanciesHandler(w http.ResponseWriter, r *http.Request) {

	// сначала проверяем указан ли ID, если да, то возвращаем конкретную вакансию
	// в стандартной библиотеки нет функционала для получения path params, поэтому используем TrimPrefix
	idVac := strings.TrimPrefix(r.URL.Path, "/vacancies/")
	_, err := strconv.Atoi(idVac)
	// если это число значит запросили вакансию по id
	if err == nil {
		var msg []byte
		res, isFound := elastic.Get(idVac)
		if !isFound {
			msg = []byte(`{"message":"not found"}`)
		} else {
			msg = res
		}
		_, err = w.Write(msg)
		if err != nil {
			log.Println(err)
		}
		return
	}

	var rawJson []*json.RawMessage
	var size int
	if r.URL.Query().Get("size") != "" {
		size, _ = strconv.Atoi(r.URL.Query().Get("size"))
	}
	if size == 0 {
		size = 1000
	}

	// если текст не указан - возвращаем все
	if r.URL.Query().Get("text") == "" {
		fullVacs := elastic.GetAll(size)
		for _, v := range fullVacs {
			rawJson = append(rawJson, &v.Source)
		}
	} else {
		rawJson = elastic.Search(r.URL.Query().Get("text"), size)
	}

	res, err := json.Marshal(rawJson)
	if err != nil {
		log.Println(err)
	}
	if res == nil || len(res) == 0 || string(res) == ("null") {
		res = []byte(`{"message":"not found"}`)
	}
	_, err = w.Write(res)
	if err != nil {
		log.Println(err)
	}
}
