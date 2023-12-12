package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	tele "gopkg.in/telebot.v3"
)

var HELP = `This bot helps you search and access your books collection.
To start using it, just send a book title or author name to the bot.
`

const actionDownload = "DOWNLOAD"

func NewTelegramBot(
	token string,
	authorizedUsers []string,
	service *Service,
) (*TelegramBot, error) {
	b, err := tele.NewBot(tele.Settings{
		Token:  token,
		Poller: &tele.LongPoller{Timeout: 10 * time.Second},
	})
	if err != nil {
		return nil, err
	}
	return &TelegramBot{
		service:         service,
		bot:             b,
		authorizedUsers: authorizedUsers,
	}, nil
}

type TelegramBot struct {
	service         *Service
	bot             *tele.Bot
	authorizedUsers []string
}

func (b *TelegramBot) Start() {
	handlers := b.bot.Group()
	handlers.Use(b.denyUnknownUsersMiddleware)
	handlers.Handle(tele.OnText, b.handleMessage)
	handlers.Handle(&tele.Btn{Unique: actionDownload}, b.handleDownloadCmd)
	handlers.Handle("/help", b.handleHelpCmd)
	b.bot.Start()
}

func (b *TelegramBot) Stop() {
	b.bot.Stop()
}

func (b *TelegramBot) handleHelpCmd(c tele.Context) error {
	return c.Send(HELP, "Markdown")
}

func (b *TelegramBot) handleMessage(c tele.Context) error {
	text := c.Text()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	books, err := b.service.Search(ctx, text)
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return c.Send("Sorry, the search took too long, please try again later")
	case err != nil:
		log.Printf("[bot] error searching for %q: %v", text, err)
		return c.Send("Sorry, something went wrong, please try again later")
	case len(books) == 0:
		return c.Send("Sorry, no books found")
	case len(books) > 100:
		return c.Send(fmt.Sprintf("Sorry, found too many books: %d. Please try to narrow down your search", len(books)))
	default:
		return b.sendBooks(c, books)
	}
}

func (b *TelegramBot) sendBooks(c tele.Context, metas []*BookMetadata) error {
	for _, m := range metas {
		bookTitle := fmt.Sprintf("[%s %s] - %s", m.AuthorLastName, m.AuthorFirstName, m.Title)
		if m.Date != "" {
			bookTitle += fmt.Sprintf(" (%s)", m.Date)
		}

		markup := &tele.ReplyMarkup{}
		epubBtn := tele.Btn{
			Unique: actionDownload,
			Text:   ".epub",
			Data:   fmt.Sprintf("epub|%d", m.ID),
		}
		markup.Inline(markup.Row(epubBtn))

		if err := c.Send(bookTitle, markup); err != nil {
			return err
		}
	}
	return nil
}

func (b *TelegramBot) handleDownloadCmd(c tele.Context) error {
	data := c.Callback().Data
	parts := strings.Split(data, "|")
	if len(parts) != 2 || parts[0] != "epub" {
		log.Printf("[bot] invalid callback data: %q", data)
		_ = c.Send("Sorry, something went wrong, please try again later")
		return c.Respond()
	}
	id, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		log.Printf("[bot] invalid callback data: %q", data)
		_ = c.Send("Sorry, something went wrong, please try again later")
		return c.Respond()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	reader, cleanup, err := b.service.GetBook(ctx, id, EPUB)
	if err != nil {
		log.Printf("[bot] error getting book: %v", err)
		_ = c.Send("Sorry, something went wrong, please try again later")
		return c.Respond()
	}
	defer cleanup()

	if err := c.Send(&tele.Document{
		File:     tele.FromReader(reader),
		FileName: fmt.Sprintf("%d.epub", id),
	}); err != nil {
		log.Printf("[bot] error sending book: %v", err)
		_ = c.Send("Sorry, something went wrong, please try again later")
		return c.Respond()
	}

	return c.Respond()
}

func (b *TelegramBot) denyUnknownUsersMiddleware(next tele.HandlerFunc) tele.HandlerFunc {
	return func(c tele.Context) error {
		userID := c.Sender().ID
		userName := c.Sender().Username

		for _, u := range b.authorizedUsers {
			if u == userName || u == strconv.FormatInt(userID, 10) {
				return next(c)
			}
		}

		return c.Send("This is a private bot. If you know the author, ask them to invite you, otherwise just have a great day!")
	}
}
