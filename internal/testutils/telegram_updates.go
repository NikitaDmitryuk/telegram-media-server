package testutils

import tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

func CommandUpdate(chatID, userID int64, userName, text string) *tgbotapi.Update {
	return &tgbotapi.Update{Message: &tgbotapi.Message{
		From: &tgbotapi.User{ID: userID, UserName: userName},
		Chat: &tgbotapi.Chat{ID: chatID},
		Text: text,
	}}
}

func TextUpdate(chatID, userID int64, userName, text string) *tgbotapi.Update {
	return CommandUpdate(chatID, userID, userName, text)
}

func CallbackUpdate(chatID, userID int64, userName, data string, messageID int) *tgbotapi.Update {
	return &tgbotapi.Update{CallbackQuery: &tgbotapi.CallbackQuery{
		ID:   "callback-id",
		From: &tgbotapi.User{ID: userID, UserName: userName},
		Data: data,
		Message: &tgbotapi.Message{
			MessageID: messageID,
			Chat:      &tgbotapi.Chat{ID: chatID},
		},
	}}
}

func TorrentDocumentUpdate(chatID, userID int64, userName, fileID, fileName string) *tgbotapi.Update {
	return &tgbotapi.Update{Message: &tgbotapi.Message{
		From: &tgbotapi.User{ID: userID, UserName: userName},
		Chat: &tgbotapi.Chat{ID: chatID},
		Document: &tgbotapi.Document{
			FileID:   fileID,
			FileName: fileName,
		},
	}}
}
