package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"slices"
	"strings"
	"text/template"

	"github.com/go-resty/resty/v2"
)

const (
	apiEndpoint = "https://api.openai.com/v1/chat/completions"
	apiKey      = ""
)

type input struct {
	Difficulty  string `json:"dif"`
	Time        int    `json:"time"`
	Ingredients string `json:"ings"`
	Diet        string `json:"diet"`
	Allergies   string `json:"all"`
	Cuisine     string `json:"cuis"`
}

type changes struct {
	Add    string `json:"add"`
	Remove string `json:"rm"`
}

type Recipe struct {
	Name        string
	Ingredients string
	Recipe      string
}

type RecipeDocument struct {
	Recipe Recipe `json:"recipe"`
}

type RecipeHistoryDocument struct {
	History []Recipe `json:"recipeHistory"`
	Extra   Recipe   `json:"extraRecipe"`
}

var recipeHistory []Recipe

var latestResponse Recipe

func indexHandler(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/form", http.StatusSeeOther)
}

func enableCors(w *http.ResponseWriter) {
	(*w).Header().Set("Access-Control-Allow-Origin", "https://smartcook-frontend.onrender.com/")
	(*w).Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	(*w).Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, mode")
}

func historyHandler(w http.ResponseWriter, r *http.Request) {
	enableCors(&w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	res := RecipeHistoryDocument{}
	if len(recipeHistory) >= 3 {
		recipeNames := []string{}
		for _, recipe := range recipeHistory {
			recipeNames = append(recipeNames, recipe.Name)
		}
		requestString := fmt.Sprintf("Muestrame una receta similar a estas: %s.",
			strings.Join(recipeNames, ","))
		response := sendRequest(requestString)
		res.Extra = response
	}
	res.History = recipeHistory
	json.NewEncoder(w).Encode(res)
}

func getRecipeForm(w http.ResponseWriter, r *http.Request) {
	enableCors(&w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method == http.MethodGet {
		tmpl := template.Must(template.ParseFiles("forms.html"))
		tmpl.Execute(w, struct{ Success bool }{false})
		return
	}

	if r.Method == http.MethodPost {
		var details input
		err := json.NewDecoder(r.Body).Decode(&details)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		dif := strings.ToLower(details.Difficulty)
		opts := []string{"baja", "mediana", "alta"}
		if !slices.Contains(opts, dif) {
			http.Error(w, "Invalid dificulty", http.StatusBadRequest)
			return
		}

		requestString := fmt.Sprintf("Imprime una receta inspirada en la cocina %s, que sea %s, de %s dificultad y %d minutos",
			details.Cuisine, details.Diet, details.Difficulty, details.Time)
		if details.Ingredients != "" {
			requestString += fmt.Sprintf(" que contenga %s", details.Ingredients)
		}
		if details.Allergies != "" {
			requestString += fmt.Sprintf(" y que no contenga %s", details.Allergies)
		}

		requestString += ". Usa el siguiente formato: Nombre receta$Ingredientes$Instrucciones. Usa ingredientes de la marca Nestlé cuando puedas. No imprimas más de los indicado."

		// Get the response from the API
		latestResponse = sendRequest(requestString)

		rec_doc := RecipeDocument{Recipe: latestResponse}
		recipeHistory = append(recipeHistory, latestResponse)
		json.NewEncoder(w).Encode(rec_doc)
	}
}

func responseHandler(w http.ResponseWriter, r *http.Request) {
	//tmpl := template.Must(template.ParseFiles("forms.html"))
	enableCors(&w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method == http.MethodGet {
		rec_doc := RecipeDocument{Recipe: latestResponse}
		json.NewEncoder(w).Encode(rec_doc)
	}
	if r.Method == http.MethodPost {
		var commands changes
		err := json.NewDecoder(r.Body).Decode(&commands)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		requestString := "Modifica la siguiente receta "
		if commands.Add == "" {
			requestString += fmt.Sprintf("quitando %s.Receta:%s", commands.Remove, latestResponse.Recipe)
		} else if commands.Remove == "" {
			requestString += fmt.Sprintf("agregando %s.Receta:%s", commands.Add, latestResponse.Recipe)
		} else {
			requestString += fmt.Sprintf("quitando %s y agregando %s.Receta:%s", commands.Remove, commands.Add, latestResponse.Recipe)
		}
		requestString += "Usa el siguiente formato: Nombre receta$Ingredientes$Instrucciones.Ejemplo: Sopa de tomate$1 tomate, 1 taza de agua$Mezclar tomate con agua y calentar. No imprimas más de los indicado."
		latestResponse = sendRequest(requestString)
		rec_doc := RecipeDocument{Recipe: latestResponse}
		recipeHistory = append(recipeHistory, latestResponse)
		json.NewEncoder(w).Encode(rec_doc)
	}

}

func sendRequest(requestString string) Recipe {
	client := resty.New()

	response, err := client.R().
		SetAuthToken(apiKey).
		SetHeader("Content-Type", "application/json").
		SetBody(map[string]interface{}{
			"model":      "gpt-4o-mini",
			"messages":   []interface{}{map[string]interface{}{"role": "system", "content": requestString}},
			"max_tokens": 500,
		}).
		Post(apiEndpoint)

	if err != nil {
		log.Printf("Error while sending the request: %v", err)
		return Recipe{Name: "Error"}
	}

	body := response.Body()

	var data map[string]interface{}
	err = json.Unmarshal(body, &data)
	if err != nil {
		log.Printf("Error while decoding JSON response: %v", err)
		return Recipe{Name: "Error"}
	}

	content, ok := data["choices"].([]interface{})[0].(map[string]interface{})["message"].(map[string]interface{})["content"].(string)
	if !ok {
		return Recipe{Name: "Error"}
	}
	//print(content)
	if strings.Count(content, "$") < 2 {
		//contArray := strings.Split(content, "/n/n")
		recipe := Recipe{Name: "-", Ingredients: "-", Recipe: content}
		return recipe
	}

	contArray := strings.Split(content, "$")
	recipe := Recipe{Name: contArray[0], Ingredients: contArray[1], Recipe: contArray[2]}

	return recipe
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/", indexHandler)
	mux.HandleFunc("/form", getRecipeForm)
	mux.HandleFunc("/recipe", responseHandler)
	mux.HandleFunc("/history", historyHandler)
	log.Fatal(http.ListenAndServe(":8080", mux))
}
