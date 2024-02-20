package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

type GoogleSearchResult struct {
	Title      string `json:"title"`
	Link       string `json:"link"`
	Location   string `json:"location"`
	Keyword    string `json:"keyword"`
	HTML       string `json:"html"`
	SearchTerm string `json:"searchTerm"`
}


func processGoogleResponseAndSaveToMongo(w http.ResponseWriter, searchTerm string) {
	
	googleURL := fmt.Sprintf("https://www.google.com/search?q=%s", url.QueryEscape(searchTerm))
	log.Println("URL google", googleURL)

	resp, err := http.Get(googleURL)
	if err != nil {
		http.Error(w, "Erro ao enviar solicitação de pesquisa para o Google", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, "Erro ao ler o HTML da resposta do Google", http.StatusInternalServerError)
		return
	}
	html := string(body)

	searchResults := extractDataFromGooglePage(html, searchTerm)

	client, err := mongo.NewClient(options.Client().ApplyURI("mongodb://localhost:27017"))
	if err != nil {
		log.Fatal("Erro ao criar cliente do MongoDB:", err)
	}
	defer client.Disconnect(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err = client.Connect(ctx)
	if err != nil {
		log.Fatal("Erro ao conectar ao MongoDB:", err)
	}

	err = client.Ping(ctx, readpref.Primary())
	if err != nil {
		log.Fatal("Erro ao pingar o MongoDB:", err)
	}

	db := client.Database("collections")
	collection := db.Collection("data")

	log.Println("dados chegando para gravar ctx, searchResults", ctx, searchResults)
	_, err = collection.InsertMany(ctx, searchResults)
	if err != nil {
		log.Fatal("Erro ao inserir documentos no MongoDB:", err)
	}

	waitChan := make(chan struct{})
	go func() {
		defer close(waitChan)
		<-ctx.Done()
	}()

	
	<-waitChan

	
	jsonResponse := map[string]string{"message": "Resultados da pesquisa salvos no MongoDB com sucesso!"}
	json.NewEncoder(w).Encode(jsonResponse)
}

func extractDataFromGooglePage(html, searchTerm string) []interface{} {
	var searchResults []interface{}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		log.Println("Erro ao analisar a página HTML:", err)
		return searchResults
	}

	doc.Find("a").Each(func(i int, s *goquery.Selection) {
		title := s.Text()
		link, _ := s.Attr("href")
		location := s.Find(".TbwUpd").Text() 
		keyword := searchTerm

		html := s.Text()

		searchResult := GoogleSearchResult{
			Title:      title,
			Link:       link,
			Location:   location,
			Keyword:    keyword,
			HTML:       html,
			SearchTerm: searchTerm,
		}
		searchResults = append(searchResults, searchResult)
	})

	return searchResults
}

func handleSearchRequest(w http.ResponseWriter, r *http.Request) {
	var searchParams map[string]interface{}
	err := json.NewDecoder(r.Body).Decode(&searchParams)
	if err != nil {
		http.Error(w, "Erro ao decodificar parâmetros", http.StatusBadRequest)
		return
	}

	searchTerm, ok := searchParams["term"].(string)
	if !ok {
		http.Error(w, "Termo de pesquisa ausente ou inválido", http.StatusBadRequest)
		return
	}

	processGoogleResponseAndSaveToMongo(w, searchTerm)
}

func main() {
	http.HandleFunc("/", handleSearchRequest)
	fmt.Println("Servidor do robô de processamento em Golang iniciado na porta :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

