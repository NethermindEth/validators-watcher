package alerts

import (
	"github.com/sirupsen/logrus"
	"github.com/slack-go/slack"
	"time"
)

type SlackAlertSender struct {
	channelID string
	client    *slack.Client
}

func (s SlackAlertSender) SendAlert(alert string) error {
	// Create the Slack attachment that we will send to the channel
	attachment := slack.Attachment{
		Pretext: "ALERT",
		Text:    alert,
		// Color Styles the Text, making it possible to have like Warnings etc.
		Color: "#ff0000",
		// Fields are Optional extra data!
		Fields: []slack.AttachmentField{
			{
				Title: "Date",
				Value: time.Now().String(),
			},
		},
	}
	// PostMessage will send the message away.
	// First parameter is just the channelID, makes no sense to accept it
	_, timestamp, err := s.client.PostMessage(
		s.channelID,
		// uncomment the item below to add a extra Header to the message, try it out :)
		//slack.MsgOptionText("New message from bot", false),
		slack.MsgOptionAttachments(attachment),
	)

	if err != nil {
		return err
	}
	logrus.Warn("Message sent at %s", timestamp)
	return nil
}

func NewSlackAlertSender(token, channelID string) *SlackAlertSender {
	// Create a new client to slack by giving token
	// Set debug to true while developing
	client := slack.New(token, slack.OptionDebug(true))
	return &SlackAlertSender{
		client:    client,
		channelID: channelID,
	}
}
