package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"rest-server-chi/globals"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
)

type Credentials struct {
	User string `json:"user"`
	Pwd  string `json:"pwd"`
}

type Query struct {
	Term       string `json:"term"`
	Start_time string `json:"start_time,omitempty"`
	End_time   string `json:"end_time,omitempty"`
}

type ZincSearchQuery struct {
	Search_type string   `json:"search_type"`
	Query       Query    `json:"query"`
	Sort_fields []string `json:"sort_fields"`
	From        uint     `json:"from"`
	Max_results uint     `json:"max_results"`
	Source      []string `json:"_source"`
}

type ClientReq struct {
	Field  string   `json:"field,omitempty"`
	Query  string   `json:"query"`
	Sort   []string `json:"sort"`
	Fields []string `json:"fields"`
	Range  []uint   `json:"range"`
}

// TODO: ESTRUCTURAS PARA ARMAR RESPUESTA A CLIENTE
type Hit struct {
	Index  string            `json:"_index"`
	Source map[string]string `json:"_source"`
}

type Total struct {
	Value uint `json:"value"`
}

type Hits struct {
	Total Total `json:"total"`
	Hits  []Hit `json:"hits"`
}

type zincResult struct {
	Timedout bool `json:"timed_out"`
	Hits     Hits `json:"hits"`
}

type ServerResp struct {
	Timedout bool                `json:"timeout"`
	Total    uint                `json:"total"`
	Results  []map[string]string `json:"results"`
}

func main() {

	//Iniciar Router
	r := chi.NewRouter()

	//Middlewares
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"https://*", "http://*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Authorization", "Content-Type"},
		AllowCredentials: true,
	}))
	r.Use(middleware.AllowContentType("application/json"))
	r.Use(middleware.Logger)

	//Rutas
	r.Get("/api/busqueda", searchController)

	//Levantar server
	fmt.Printf("Rest server corriendo en: http://localhost:%v \n", globals.PORT)
	http.ListenAndServe(":"+globals.PORT, r)

}

func searchController(w http.ResponseWriter, r *http.Request) {

	w.Header().Set("Content-Type", "application/json")

	//Obtener header de Auth
	auth64 := r.Header.Get("Authorization")

	//Validación de auth
	credentials, err := isAuth(auth64)
	if err != nil {
		msg := make(map[string]string)
		msg["msg"] = err.Error()

		ansJson, err := json.Marshal(msg)
		if err != nil {
			fmt.Println(err)
		}

		w.WriteHeader(401)
		w.Write(ansJson)

		return
	}

	//Obtener parametros
	body, err := io.ReadAll(r.Body)

	if err != nil {
		msg := make(map[string]string)
		msg["msg"] = err.Error()

		ansJson, err := json.Marshal(msg)
		if err != nil {
			fmt.Println(err)
		}

		w.WriteHeader(400)
		w.Write(ansJson)

		return
	}

	req := ClientReq{}
	json.Unmarshal([]byte(body), &req)
	fmt.Printf("CLIENT QUERY: %v \n", req)

	//Construcción de petición a API de ZincSearch
	var search_type, querystring string
	if len(req.Field) > 0 {
		search_type = "querystring"
		querystring = req.Field + `:"` + req.Query + `"`
	} else {
		search_type = "matchphrase"
		querystring = req.Query
	}

	resp := ZincSearchQuery{
		Search_type: search_type,
		Query: Query{
			Term: querystring,
		},
		Sort_fields: req.Sort,     //Pasar campo para ordenar
		From:        req.Range[0], //Pasar limite inferior
		Max_results: req.Range[1], //Pasar limite superior
		Source:      req.Fields,   //Pasar campos a retornar
	}
	respJson, _ := json.Marshal(resp)

	//Petición a api de Zincsearch
	privateEndpoint := globals.ZINC_ENDPOINT + "/" + globals.ZINC_INDEX + "/_search" //RUTA A UTILIZAR CON LA API INTERNA
	zincReq, err := http.NewRequest("POST", privateEndpoint, strings.NewReader(string(respJson)))
	if err != nil {
		fmt.Println(err)
	}

	zincReq.SetBasicAuth(credentials.User, credentials.Pwd)
	zincReq.Header.Set("Content-type", "application/json")

	zincResp, err := http.DefaultClient.Do(zincReq)
	if err != nil {
		fmt.Println(err)
	}
	defer zincResp.Body.Close()

	zincBody, _ := io.ReadAll(zincResp.Body)

	zincResult := zincResult{}
	json.Unmarshal(zincBody, &zincResult)

	results := make([]map[string]string, 0)
	for _, result := range zincResult.Hits.Hits {
		results = append(results, result.Source)
	}

	serverResp := ServerResp{zincResult.Timedout, zincResult.Hits.Total.Value, results}

	serverRespJSON, err := json.Marshal(serverResp)
	if err != nil {
		fmt.Println(err)
	}

	//Enviar respuesta a cliente
	w.Write(serverRespJSON)

}

// Función para validar credenciales de Zincsearch
func isAuth(auth64 string) (Credentials, error) {
	auth, err := base64.StdEncoding.DecodeString(auth64)
	authSlice := strings.Split(string(auth), ":")

	if len(auth) == 0 {
		return Credentials{}, errors.New("no credentials")
	}

	credentials := Credentials{authSlice[0], authSlice[1]}

	if err != nil || credentials.User != globals.ZINC_USER || credentials.Pwd != globals.ZINC_PWD {
		return credentials, errors.New("incorrect credentials")
	}

	return credentials, nil

}
