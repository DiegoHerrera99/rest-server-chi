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

type ServerResp struct {
	Search_type string      `json:"search_type"`
	Query       Query       `json:"query"`
	Sort_fields []string    `json:"sort_fields"`
	From        uint        `json:"from"`
	Max_results uint        `json:"max_results"`
	Source      []string    `json:"_source"`
	Auth        Credentials `json:"auth"`
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
	//privateEndpoint := globals.ZINC_ENDPOINT + "/" + globals.ZINC_INDEX + "/_search" RUTA A UTILIZAR CON LA API INTERNA
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

	req := make(map[string]string)
	json.Unmarshal([]byte(body), &req)
	field := req["field"]
	query := req["query"]

	//Construcción de petición a API de ZincSearch
	var search_type, querystring string
	if len(field) > 0 {
		search_type = "querystring"
		querystring = field + `:"` + query + `"`
	} else {
		search_type = "matchphrase"
		querystring = query
	}

	resp := ServerResp{
		Search_type: search_type,
		Query: Query{
			Term: querystring,
		},
		Sort_fields: []string{"-date"},
		From:        0,
		Max_results: 20,
		Source:      []string{"from", "subject", "date"},
		Auth:        credentials,
	}
	respJson, _ := json.Marshal(resp)

	//TODO: Petición a api de Zincsearch

	//Enviar respuesta a cliente
	w.Write(respJson)

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
