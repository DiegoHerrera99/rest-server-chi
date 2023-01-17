package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"rest-server-chi/globals"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
)

type ClientReq struct {
	Field string `json:"field,omitempty"`
	Query string `json:"query"`
	Auth  string `json:"auth"`
}

type ErrAns struct {
	Msg string `json:"msg"`
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
	r.Post("/api/busqueda", searchController)

	//Levantar server
	fmt.Printf("Rest server corriendo en: http://localhost:%v \n", globals.PORT)
	http.ListenAndServe(":"+globals.PORT, r)

}

func searchController(w http.ResponseWriter, r *http.Request) {
	//privateEndpoint := globals.ZINC_ENDPOINT + "/" + globals.ZINC_INDEX + "/_search" RUTA A UTILIZAR CON LA API INTERNA
	w.Header().Set("Content-Type", "application/json")

	//Obtener header de Auth
	auth64 := r.Header.Get("Authorization")
	auth, err := base64.StdEncoding.DecodeString(auth64)

	//Validación de auth
	//TODO: Validar contra credenciales de Zincsearch
	if err != nil {
		ans := ErrAns{Msg: "Bad Request | Incorrect Credentials"}

		ansJson, err := json.Marshal(ans)

		if err != nil {
			fmt.Println(err)
		}

		w.WriteHeader(401)
		w.Write(ansJson)

		return
	}

	if len(auth) == 0 {
		ans := ErrAns{Msg: "Bad Request | No Credentials"}

		ansJson, err := json.Marshal(ans)

		if err != nil {
			fmt.Println(err)
		}

		w.WriteHeader(400)
		w.Write(ansJson)

		return
	}

	//Obtener parametros
	fieldParam := r.URL.Query().Get("field")
	queryParam := r.URL.Query().Get("query")

	//TODO: Obtener y enviar respuesta
	resp := ClientReq{fieldParam, queryParam, string(auth)}

	jsonResp, err := json.Marshal(resp)
	if err != nil {
		fmt.Println(err)
	}

	w.Write(jsonResp)

}
