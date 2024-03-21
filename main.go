package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"strings"
)

var (
	buildCommand     string
	restartCommand   string
	prodUrl          string
	appName          string
	telegramBotToken string
	chatID           int64
	port             string
)

type (
	TelegramMessage struct {
		ChatID                int64                `json:"chat_id"`
		Text                  string               `json:"text"`
		ParseMode             string               `json:"parse_mode,omitempty"`
		ReplyMarkup           *ReplyKeyboardMarkup `json:"reply_markup,omitempty"`
		DisableWebPagePreview bool                 `json:"disable_web_page_preview,omitempty"`
	}

	ReplyKeyboardMarkup struct {
		InlineKeyboard [][]InlineKeyboardButton `json:"inline_keyboard"`
	}

	InlineKeyboardButton struct {
		Text string `json:"text"`
		URL  string `json:"url"`
	}

	WebhookPayload struct {
		Repository struct {
			HtmlURL string `json:"html_url"`
		} `json:"repository"`
		Sender struct {
			HtmlURL string `json:"html_url"`
		} `json:"sender"`
		Commits []struct {
			Message string `json:"message"`
			Author  struct {
				Name     string `json:"name"`
				Username string `json:"Username"`
			} `json:"author"`
			Url       string `json:"url"`
			Timestamp string `json:"timestamp"`
		} `json:"commits"`
	}
)

func main() {
	flag.StringVar(&appName, "name", "", "Command to build the application")
	flag.StringVar(&buildCommand, "pull-cmd", "", "Command to build the application")
	flag.StringVar(&restartCommand, "restart-cmd", "", "Command to restart the application")
	flag.StringVar(&prodUrl, "url", "", "URL of the production server")
	flag.StringVar(&telegramBotToken, "token", "", "Telegram bot token")
	flag.Int64Var(&chatID, "chat", 0, "Chat ID of the Telegram channel")
	flag.StringVar(&port, "port", "8080", "Port to run the server on")
	flag.Parse()
	if buildCommand == "" || restartCommand == "" || prodUrl == "" || appName == "" || telegramBotToken == "" || chatID == 0 {
		log.Println("Required flags: -name, -pull-cmd, -restart-cmd, -url, -token, -chat")
		return
	}
	http.HandleFunc("/", handleHello)
	http.HandleFunc("/webhook", handleWebhook)
	log.Println("Server started at :8080")
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func handleHello(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, "Hello!")
}

func handleWebhook(w http.ResponseWriter, r *http.Request) {
	var payload WebhookPayload
	err := json.NewDecoder(r.Body).Decode(&payload)
	if err != nil {
		http.Error(w, "Error decoding JSON", http.StatusBadRequest)
		log.Printf("Error decoding JSON: %v\n", err)
		return
	}

	authorName := payload.Commits[0].Author.Name
	committerName := payload.Commits[0].Author.Username
	userProfileURL := payload.Sender.HtmlURL
	message := strings.ReplaceAll(payload.Commits[0].Message, "_", "\\_")
	commitURL := payload.Commits[0].Url
	commitDate := payload.Commits[0].Timestamp

	formattedMessage := fmt.Sprintf("*Author:* `%s`\n*Committer:* [%s](%s)\n*Message:* `%s`\n*Time Stamp:* `%s`",
		authorName, committerName, userProfileURL, message, commitDate)

	replyMarkup := &ReplyKeyboardMarkup{
		InlineKeyboard: [][]InlineKeyboardButton{
			{{Text: "View Commit", URL: commitURL}},
		},
	}

	sendMessageToTelegram(formattedMessage, replyMarkup)

	executeCommand(buildCommand, "Application is being built from the latest commit", commitURL, "View Commit")
	executeCommand(restartCommand, "Application has been restarted", prodUrl, "View Application")

	if !checkProdUrl(prodUrl) {
		sendMessageToTelegram("Production URL is not reachable", &ReplyKeyboardMarkup{
			InlineKeyboard: [][]InlineKeyboardButton{{{Text: "View Application", URL: prodUrl}}},
		})
		w.WriteHeader(http.StatusOK)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func checkProdUrl(url string) bool {
	resp, err := http.Get(url)
	if err != nil {
		log.Printf("Error checking production URL: %v\n", err)
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func sendMessageToTelegram(message string, replyMarkup *ReplyKeyboardMarkup) {
	telegramAPIURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", telegramBotToken)
	message = fmt.Sprintf("Update for *%s*:\n\n%s", appName, message)
	telegramMessage := TelegramMessage{
		ChatID:                chatID,
		Text:                  message,
		ParseMode:             "Markdown",
		DisableWebPagePreview: true,
	}

	if replyMarkup != nil {
		telegramMessage.ReplyMarkup = replyMarkup
	}

	telegramMessageJSON, err := json.Marshal(telegramMessage)
	if err != nil {
		log.Printf("Error marshalling Telegram message: %v\n", err)
		return
	}

	resp, err := http.Post(telegramAPIURL, "application/json", bytes.NewBuffer(telegramMessageJSON))
	if err != nil {
		log.Printf("Error sending message to Telegram: %v\n", err)
		return
	}
	defer resp.Body.Close()
}

func executeCommand(cmdString, successMessage, url, text string) {
	if cmdString == "" {
		log.Println("No command provided")
		return
	}

	cmd := exec.Command("bash", "-c", cmdString)
	err := cmd.Run()
	if err != nil {
		sendMessageToTelegram(fmt.Sprintf("Error executing command: %v", err), nil)
		return
	}
	sendMessageToTelegram(successMessage, &ReplyKeyboardMarkup{
		InlineKeyboard: [][]InlineKeyboardButton{{{Text: text, URL: url}}},
	})
}
