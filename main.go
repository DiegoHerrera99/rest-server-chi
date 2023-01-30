package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"rest-server-chi/globals"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
)

// DEFINICIÓN DE TYPES
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
	Sort   []string `json:"sort,omitempty"`
	Fields []string `json:"fields,omitempty"`
	Range  []uint   `json:"range,omitempty"`
}

type Hit struct {
	Source map[string]string `json:"_source"`
	Id     string            `json:"_id"`
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
	Total   uint                `json:"total"`
	Results []map[string]string `json:"results"`
}

func main() {

	//GET PORT
	var port string
	flag.StringVar(&port, "port", globals.PORT, "specify port number")
	flag.StringVar(&port, "p", globals.PORT, "specify port number")
	flag.Parse()

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

	//Crear servidor de contenido estatico (Frontend - Vue3)
	workDir, _ := os.Getwd()
	filesDir := http.Dir(filepath.Join(workDir, "public"))
	FileServer(r, "/", filesDir)

	//Levantar server
	fmt.Printf("Email reader corriendo en: http://localhost:%v/ \n", port)
	http.ListenAndServe(":"+port, r)

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

	//CREACIÓN DE REQUEST LÉGIBLE
	req := ClientReq{}
	json.Unmarshal([]byte(body), &req)
	fmt.Printf("CLIENT QUERY: %v \n", req)

	//SANITIZACIÓN DE REQUEST -- VALIDACIONES
	if len(req.Query) == 0 {
		msg := make(map[string]string)
		msg["msg"] = "empty query"

		ansJson, err := json.Marshal(msg)
		if err != nil {
			fmt.Println(err)
		}

		w.WriteHeader(400)
		w.Write(ansJson)

		return
	}

	if len(req.Fields) == 0 {
		req.Fields = []string{"subject", "from", "date", "to", "body"}
	}

	if len(req.Range) == 0 {
		req.Range = []uint{0, 10}
	}

	if len(req.Sort) == 0 {
		req.Sort = []string{"-date"}
	}

	//CONSTRUCCIÓN DE PETICIÓN DE API ZINCSEARCH
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
	respJson, err := json.Marshal(resp)
	if err != nil {
		fmt.Println(err)
	}

	//PETICIÓN A API ZINCSEARCH
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

	//LECTURA DE RESPUESTA
	zincBody, _ := io.ReadAll(zincResp.Body)

	//OBTENCION DE DATOS DE ZINCSEARCH (CONVERSIÓN DE JSON A STRUCT)
	zincResult := zincResult{}
	json.Unmarshal(zincBody, &zincResult)

	//VALIDACIÓN DE ERRORES
	if zincResult.Timedout {
		msg := make(map[string]string)
		msg["msg"] = "request timeout"

		ansJson, err := json.Marshal(msg)
		if err != nil {
			fmt.Println(err)
		}

		w.WriteHeader(408)
		w.Write(ansJson)

		return
	}

	//OBTENER RESULTADOS DE BUSQUEDA
	results := make([]map[string]string, 0)
	for _, result := range zincResult.Hits.Hits {
		result.Source["id"] = result.Id
		results = append(results, result.Source)
	}

	//FORMATEO DE RESPUESTA DE ZINC A RESPUESTA DE NUESTRO SERVER
	serverResp := ServerResp{zincResult.Hits.Total.Value, results}

	serverRespJSON, err := json.Marshal(serverResp)
	if err != nil {
		fmt.Println(err)
	}

	//ENVIAR RESPUESTA A CLIENTE
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

// FileServer conveniently sets up a http.FileServer handler to serve
// static files from a http.FileSystem.
func FileServer(r chi.Router, path string, root http.FileSystem) {
	if strings.ContainsAny(path, "{}*") {
		panic("FileServer does not permit any URL parameters.")
	}

	if path != "/" && path[len(path)-1] != '/' {
		r.Get(path, http.RedirectHandler(path+"/", 301).ServeHTTP)
		path += "/"
	}
	path += "*"

	r.Get(path, func(w http.ResponseWriter, r *http.Request) {
		rctx := chi.RouteContext(r.Context())
		pathPrefix := strings.TrimSuffix(rctx.RoutePattern(), "/*")
		fs := http.StripPrefix(pathPrefix, http.FileServer(root))
		fs.ServeHTTP(w, r)
	})
}
