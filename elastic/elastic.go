package elastic

// пакет для работы с elastic search

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/olivere/elastic/v7"
	"log"
	"os"
	"regexp"
	"time"
)

// вакансии за страницу
type VacanciesPerPage struct {
	PerPage int       `json:"per_page"`
	Items   Vacancies `json:"items"`
	Page    int       `json:"page"`
	Pages   int       `json:"pages"`
	Found   int       `json:"found"`
}

// краткий вид вакансии
type Vacancy struct {
	Name string `json:"name"`
	Id   string `json:"id"`
}

// Полный вид вакансии с описанием
type FullVacancy struct {
	Id          string `json:"id"`
	Description string `json:"description"`
	KeySkills   []struct {
		Name string `json:"name"`
	} `json:"key_skills"`
	Name string `json:"name"`
}

type FullVacancies []*FullVacancy
type Vacancies []*Vacancy

var esclient *elastic.Client

var regexpDeleteTags = regexp.MustCompile(`(\<(\/?[^>]+)>)`) // для удалением тэгов

func Init() {

	esclientURL := os.Getenv("ES_HOST")
	if esclientURL == "" {
		esclientURL = "http://localhost:9200" // url по умолчанию
	}

	// пробуем подключиться к эластику, возможны задержки, поэтому пробуем несколько раз
	for i := 0; i < 10; i++ {

		connect(esclientURL)

		// ping the service
		_, _, err := esclient.Ping(esclientURL).Do(context.Background())
		if err != nil {
			if i > 6 {
				log.Println("Can't ping elastic: ", err)
			}
		} else {
			fmt.Println("the connection to elastic is successful: ", esclientURL)
			break
		}

		time.Sleep(4 * time.Second)
	}

}

// существует ли индекс
func IsIndexExist(index string) bool {
	isExist, _ := esclient.IndexExists(index).Do(context.Background())
	return isExist
}

// подключение к elastic
func connect(esclientURL string) {
	var err error
	fmt.Println("Trying connect to elastic: ", esclientURL)
	esclient, err = elastic.NewClient(elastic.SetURL(esclientURL),
		elastic.SetSniff(false),
		elastic.SetHealthcheck(false))
	if err != nil {
		fmt.Println("Error initializing : ", err)
	}
}

// множественная вставка полных вакансий
func (fullVacancies FullVacancies) InsertMany() {
	bulkRequest := esclient.Bulk()
	for _, v := range fullVacancies {
		v.Description = regexpDeleteTags.ReplaceAllString(v.Description, "")
		req := elastic.NewBulkIndexRequest().Index("full_vacancies").Id(v.Id).Doc(v)
		bulkRequest = bulkRequest.Add(req)
	}
	createBulkRequest(bulkRequest)
}

// вставка документа
func (fullVac *FullVacancy) InsertOne() {

	fullVac.Description = regexpDeleteTags.ReplaceAllString(fullVac.Description, "")

	res, err := json.Marshal(fullVac)
	if err != nil {
		log.Println(err)
	}
	_, err = esclient.Index().
		Index("full_vacancies").
		BodyString(string(res)).
		Id(fullVac.Id).
		Do(context.Background())
	if err != nil {
		log.Println(err)
	}
}

// получение всех документов по index
func GetAll(size int) []*elastic.SearchHit {

	// Get all documents
	res, err := esclient.Search().
		Index("full_vacancies").
		Size(size).
		Do(context.TODO())

	if err != nil {
		log.Println("[GetAll] can't get docs:", err)
	}

	if res != nil {
		if res.TotalHits() != 0 {
			return res.Hits.Hits
		}
	}

	return nil
}

// получение документа по id
func Get(id string) ([]byte, bool) {

	res, err := esclient.Get().
		Id(id).
		Index("full_vacancies").
		Do(context.Background())

	if err != nil {
		log.Println("cant get doc:", err)
	}

	if res != nil {
		return res.Source, true
	}
	return []byte(""), false
}

// поиск по полям
func Search(text string, size int) []*json.RawMessage {
	searchSource := elastic.NewSearchSource().
		Size(size).
		Query(elastic.NewMultiMatchQuery(text, "description", "name", "key_skills"))

	searchService := esclient.Search().Index("full_vacancies").SearchSource(searchSource)
	searchResult, err := searchService.Do(context.Background())
	if err != nil {
		log.Println(err)
	}
	if searchResult == nil {
		return nil
	}
	vacancies := make([]*json.RawMessage, len(searchResult.Hits.Hits))
	for i, hit := range searchResult.Hits.Hits {
		vacancies[i] = &hit.Source
	}
	return vacancies
}

func createBulkRequest(bulkRequest *elastic.BulkService) {
	bulkResponse, err := bulkRequest.Do(context.Background())
	if err != nil {
		log.Println("can't insert: ", err)
	}
	if bulkResponse != nil {
		if bulkResponse.Errors {
			fmt.Println("ошибки в createBulkRequest()")
		}
	}
}

// множественная вставка вакансий
//func (vacancies Vacancies) InsertMany() {
//	bulkRequest := esclient.Bulk()
//	for _, v := range vacancies {
//		req := elastic.NewBulkIndexRequest().Index("vacancies").Id(v.Id).Doc(v)
//		bulkRequest = bulkRequest.Add(req)
//	}
//	createBulkRequest(bulkRequest)
//}
