package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image/png"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mdp/qrterminal/v3"
	"github.com/skip2/go-qrcode"
	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"

	_ "github.com/mattn/go-sqlite3" // Import the SQLite3 driver
	logrus "github.com/sirupsen/logrus"
)

var client *whatsmeow.Client

func main() {
	// Setup logging
	logrus.SetLevel(logrus.DebugLevel)

	// Setup database
	container, err := sqlstore.New("sqlite3", "file:whatsmeow.db?_foreign_keys=on", nil)
	if err != nil {
		log.Fatalf("Failed to create container: %v", err)
	}

	deviceStore, err := container.GetFirstDevice()
	if err != nil {
		log.Fatalf("Failed to get device: %v", err)
	}

	// Create client
	client = whatsmeow.NewClient(deviceStore, nil)
	if client.Store.ID == nil {
		qrChannel, _ := client.GetQRChannel(context.Background())
		err = client.Connect()
		if err != nil {
			log.Fatalf("Failed to connect: %v", err)
		}

		go func() {
			for evt := range qrChannel {
				if evt.Event == "code" {
					qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
					saveQRCode(evt.Code)      // Save the QR code to be used by the API
					sendQRCodeToAPI(evt.Code) // Send the QR code to the specified API
				} else {
					log.Printf("QR Channel result: %s", evt.Event)
				}
			}
		}()
	} else {
		err = client.Connect()
		if err != nil {
			log.Fatalf("Failed to connect: %v", err)
		}
	}

	// Handle received messages and other events
	client.AddEventHandler(func(evt interface{}) {
		switch v := evt.(type) {
		case *events.Message:
			handleReceivedMessage(v)
		case *events.Connected:
			fmt.Println("Connected to WhatsApp")
		case *events.OfflineSyncCompleted:
			fmt.Println("Offline sync completed")
		case *events.LoggedOut:
			fmt.Println("Logged out")
		case *events.Disconnected:
			fmt.Println("Disconnected")
		default:
			fmt.Printf("Unhandled event: %T\n", v)
		}
	})

	// Set up Gin
	router := gin.Default()
	router.POST("/send", sendMessageHandler)
	router.GET("/qr/text", generateQRTextHandler)   // Add QR code text endpoint
	router.GET("/qr/photo", generateQRPhotoHandler) // Add QR code photo endpoint
	log.Println("Starting server on port 8080")
	router.Run(":8080")
}

// Function to handle received messages
func handleReceivedMessage(message *events.Message) {
	sender := message.Info.Sender.String()
	msg := message.Message

	if msg.GetConversation() != "" {
		fmt.Printf("Received message from %s: %s\n", sender, msg.GetConversation())
	} else if msg.GetExtendedTextMessage() != nil {
		fmt.Printf("Received extended text message from %s: %s\n", sender, msg.GetExtendedTextMessage().GetText())
	} else {
		fmt.Printf("Received a message from %s, but could not determine its type\n", sender)
	}
}

// Function to send a message
func sendMessage(client *whatsmeow.Client, jid string, text string) error {
	targetJID := types.NewJID(jid, "s.whatsapp.net")
	msgID := client.GenerateMessageID()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second) // Increased timeout to 60 seconds
	defer cancel()

	_, err := client.SendMessage(ctx, targetJID, &waProto.Message{
		Conversation: proto.String(text),
	})
	if err != nil {
		log.Printf("Failed to send message: %v", err)
		return err
	}
	fmt.Println("Message sent, ID:", msgID)
	return nil
}

// Handler to send a message
func sendMessageHandler(c *gin.Context) {
	var request struct {
		JID  string `json:"jid" binding:"required"`
		Text string `json:"text" binding:"required"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		log.Println("Failed to bind JSON:", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	log.Println("Received request to send message:", request)
	err := sendMessage(client, request.JID, request.Text)
	if err != nil {
		log.Println("Failed to send message:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	log.Println("Message sent successfully")
	c.JSON(http.StatusOK, gin.H{"status": "Message sent"})
}

// Function to save QR code to a file
func saveQRCode(code string) {
	file, err := os.Create("qrcode.txt")
	if err != nil {
		log.Printf("Failed to create QR code file: %v", err)
		return
	}
	defer file.Close()

	_, err = file.WriteString(code)
	if err != nil {
		log.Printf("Failed to write QR code to file: %v", err)
	}
}

// Function to send QR code to the specified API
func sendQRCodeToAPI(code string) {
	jsonData := map[string]string{"qr_code": code}
	jsonValue, err := json.Marshal(jsonData)
	if err != nil {
		log.Printf("Failed to marshal JSON: %v", err)
		return
	}

	resp, err := http.Post("https://devapi.courstore.com/v1/qr/for_login", "application/json", bytes.NewBuffer(jsonValue))
	if err != nil {
		log.Printf("Failed to send POST request: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("Received non-OK response from API: %v", resp.Status)
	} else {
		log.Println("QR code sent successfully to the API")
	}
}

// Handler to generate and send QR code as text
func generateQRTextHandler(c *gin.Context) {
	code, err := os.ReadFile("qrcode.txt")
	if err != nil {
		log.Printf("Failed to read QR code file: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate QR code"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"qr_code": string(code)})
}

// Handler to generate and send QR code as photo
func generateQRPhotoHandler(c *gin.Context) {
	code, err := os.ReadFile("qrcode.txt")
	if err != nil {
		log.Printf("Failed to read QR code file: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate QR code"})
		return
	}

	qr, err := qrcode.New(string(code), qrcode.Medium)
	if err != nil {
		log.Printf("Failed to generate QR code image: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate QR code"})
		return
	}

	var pngBuffer bytes.Buffer
	err = png.Encode(&pngBuffer, qr.Image(256))
	if err != nil {
		log.Printf("Failed to encode QR code image: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate QR code"})
		return
	}

	c.Header("Content-Type", "image/png")
	c.Writer.Write(pngBuffer.Bytes())
}
