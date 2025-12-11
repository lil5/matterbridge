package meshcore

import (
	"crypto"
	"errors"
	"fmt"
	"regexp"
	"strconv"

	"github.com/matterbridge-org/matterbridge/bridge"
	"github.com/matterbridge-org/matterbridge/bridge/config"
	"github.com/sirupsen/logrus"

	meshcore "github.com/grey-land/meshcore-go/meshcore"
)

var (
	htmlReplacementTag = regexp.MustCompile("<[^>]*>")
	errInvalidChannel  = errors.New("invalid channel name")
)

func InvalidChannelError(name string) error {
	return fmt.Errorf("%w: %s", errInvalidChannel, name)
}

type Bmeshcore struct {
	*bridge.Config

	c *meshcore.Application

	rooms []int8
	done  chan bool
}

func New(cfg *bridge.Config) bridge.Bridger {
	b := &Bmeshcore{Config: cfg}
	return b
}

type LogHandler struct {
	Log *logrus.Entry
}

func (b *LogHandler) OnError(err error) bool {
	b.Log.Errorf("[LogHandler] handle error: %v\n", err)
	return false
}
func (h *LogHandler) HandleFrame(frame *meshcore.Frame) bool {
	switch frame.Code() {
	case meshcore.PushCodeLogRxData: // suppress logging
		return true
	case meshcore.MessageIncomming:
	}
	return false
}

func (b *Bmeshcore) OnConnected() {
	b.Log.Infof("Connected to %s", b.GetString("Server"))
}

func (b *Bmeshcore) Connect() error {
	b.Log.Infof("Connecting %s", b.GetString("Server"))
	handler := &LogHandler{Log: b.Log}
	b.done = make(chan bool)

	var err error
	b.c, err = meshcore.NewApplication(b.GetString("SerialPath"), handler)
	if err != nil {
		return err
	}
	err =
		b.c.Init()
	if err != nil {
		return err
	}
	go b.c.Loop()

	go func() {
		for {
			select {
			case e := <-b.c.Events:
				switch e {
				case
					meshcore.ResponseContactMsgRecvV3,
					meshcore.ResponseContactMsgRecv,
					meshcore.ResponseChannelMsgRecvV3,
					meshcore.ResponseChannelMsgRecv:
					b.handleSendRemoteMessage()
				}
			case <-b.done:
				b.Log.Infof("Connection closed")
				return
			}
		}
	}()

	return nil
}

func (b *Bmeshcore) Disconnect() error {
	b.done <- true

	return nil
}

func (b *Bmeshcore) JoinChannel(channel config.ChannelInfo) error {
	channelInt, err := strconv.ParseInt(channel.Name, 10, 64)
	if err != nil {
		return err
	}
	b.c.SelectChannel(int(channelInt))

	return nil
}

func (b *Bmeshcore) Send(msg config.Message) (string, error) {
	// Standard Message Send
	if msg.Event == "" {
		content := htmlReplacementTag.ReplaceAllString(msg.Text, "")
		channelInt, err := strconv.ParseInt(msg.Channel, 10, 32)
		if err != nil {
			return "", err
		}
		b.c.SelectChannel(int(channelInt))
		err = b.c.SendMessage([]byte(content))
		if err != nil {
			b.Log.Errorf("Could not send message to room %v from %v: %v", msg.Channel, msg.Username, err)

			return "", nil
		}

		return "", nil
	}

	// Message Deletion
	if msg.Event == config.EventMsgDelete {
		// Ignore message deletion
	}

	// Message is not a type that is currently supported
	return "", nil
}

func (b *Bmeshcore) handleSendRemoteMessage() error {
	lastMsg := b.c.Messages()[len(b.c.Messages())-1]

	// find room id from last message
	for _, room := range b.rooms {
		if room == lastMsg.ChannelId {
			b.Log.Debugf("<= Message is sent to room %v", room)
			return nil
		}
	}

	content := htmlReplacementTag.ReplaceAllString(lastMsg.Content(), "")

	// hash message for unique identifier
	hash := crypto.SHA1.New()
	hash.Write([]byte(content))
	id := fmt.Sprintf("%x", hash.Sum(nil))

	remoteMessage := config.Message{
		Text:     content,
		Channel:  fmt.Sprint(lastMsg.ChannelId),
		Username: lastMsg.SenderName(),
		UserID:   string(lastMsg.PubPrefix),
		Account:  b.Account,

		ID:    id,
		Extra: map[string][]any{},
	}

	_, err := b.Send(remoteMessage)
	return err
}
