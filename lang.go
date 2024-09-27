package main

type ErrorMessages struct {
	CheckUserError          string
	UnknownFileType         string
	FileDownloadError       string
	CheckFileExistenceError string
	NotEnoughDiskSpace      string
	InvalidLoginFormat      string
	LoginError              string
	WrongPassword           string
	UnknownCommand          string
	GetMovieListError       string
	InvalidID               string
	InvalidFileID           string
	MovieNotFound           string
	GetFilesError           string
	DeleteMovieDBError      string
	DeleteFilesDBError      string
	InvalidCommandFormat    string
	MovieListError          string
	MovieCheckError         string
	TorrentClientError      string
	AddTorrentError         string
	NotEnoughSpace          string
	GetMovieError           string
	DownloadStoppedLowSpeed string
	VideoTitleError         string
	VideoDownloadError      string
	VideoExists             string
}

type InfoMessages struct {
	VideoDownloading            string
	TorrentDownloadStarted      string
	FileAlreadyExists           string
	FileAddedSuccessfully       string
	StartCommand                string
	TorrentDownloadsStopped     string
	NoMovies                    string
	LoginSuccess                string
	AllMoviesDeleted            string
	MovieDeleted                string
	DownloadVideoSuccess        string
	MovieDownloaded             string
	MovieDownloading            string
	StartDownload               string
	DownloadProgress            string
	DownloadComplete            string
	DownloadStopped             string
	StartVideoDownload          string
	FileSavedAs                 string
	VideoSuccessfullyDownloaded string
	UnknownUser                 string
}

type Messages struct {
	Error ErrorMessages
	Info  InfoMessages
}

var messages = map[string]Messages{
	"en": {
		Error: ErrorMessages{
			CheckUserError:          "An error occurred while checking the user. Please try again later",
			UnknownFileType:         "Unknown file type",
			FileDownloadError:       "An error occurred while downloading the file (files larger than 50MB cannot be uploaded)",
			CheckFileExistenceError: "An error occurred while checking file existence. Please try again",
			NotEnoughDiskSpace:      "Error downloading the torrent file (not enough disk space)",
			InvalidLoginFormat:      "Invalid command format. Use: /login [PASSWORD]",
			LoginError:              "An error occurred during login. Please try again",
			WrongPassword:           "Incorrect password",
			UnknownCommand:          "Unknown command. Use /start to get the list of available commands",
			GetMovieListError:       "An error occurred while fetching the movie list. Please try again later",
			InvalidID:               "Invalid ID",
			InvalidFileID:           "Invalid file ID",
			MovieNotFound:           "Movie not found",
			GetFilesError:           "Error retrieving files for the movie",
			DeleteMovieDBError:      "Error deleting the movie record from the database",
			DeleteFilesDBError:      "Error deleting the movie files from the database",
			InvalidCommandFormat:    "Invalid command format",
			MovieListError:          "Error getting list of movies",
			MovieCheckError:         "Error checking existence of movie",
			TorrentClientError:      "Failed to create torrent client: %v",
			AddTorrentError:         "Failed to add torrent: %v",
			NotEnoughSpace:          "Not enough space to download movie %s",
			GetMovieError:           "Error getting movie: %v",
			DownloadStoppedLowSpeed: "Download of movie %s stopped due to low download speed",
			VideoTitleError:         "Error getting video title: %s",
			VideoDownloadError:      "Error downloading video: %s",
			VideoExists:             "Video already exists or is in the process of downloading",
		},
		Info: InfoMessages{
			VideoDownloading:            "Video is downloading!",
			TorrentDownloadStarted:      "Torrent download has started!",
			FileAlreadyExists:           "File already exists",
			FileAddedSuccessfully:       "File successfully added",
			StartCommand:                "<URL> - download streaming video\n/ls - get list of files\n/rm <ID> - delete movie, all - to delete all\n/stop - stop all torrent downloads",
			TorrentDownloadsStopped:     "All torrent downloads have been stopped!",
			NoMovies:                    "The list is empty",
			LoginSuccess:                "Login successful",
			AllMoviesDeleted:            "All videos have been deleted",
			MovieDeleted:                "Movie successfully deleted",
			DownloadVideoSuccess:        "Video downloaded successfully",
			MovieDownloaded:             "ID: %d\nTitle: %s\nDownloaded: Yes\n\n",
			MovieDownloading:            "ID: %d\nTitle: %s\nDownload Percentage: %d%%\n\n",
			StartDownload:               "Download started - %s",
			DownloadProgress:            "Downloading %s: %d%%",
			DownloadComplete:            "Download of movie %s completed!",
			DownloadStopped:             "Download of movie %s stopped by request",
			StartVideoDownload:          "Starting download for URL: %s",
			FileSavedAs:                 "File will be saved as: %s",
			VideoSuccessfullyDownloaded: "Video successfully downloaded: %s",
			UnknownUser:                 "Please log in using the /login [PASSWORD] command",
		},
	},
	"ru": {
		Error: ErrorMessages{
			CheckUserError:          "Произошла ошибка при проверке пользователя",
			UnknownFileType:         "Неизвестный тип файла",
			FileDownloadError:       "Произошла ошибка при загрузке файла (загрузка файлов больше 50МБ недоступна)",
			CheckFileExistenceError: "Произошла ошибка при проверке существования файла. Пожалуйста, попробуйте снова",
			NotEnoughDiskSpace:      "Ошибка при загрузке торрента (возможно, не хватает места на диске)",
			InvalidLoginFormat:      "Неверный формат команды. Используйте: /login [PASSWORD]",
			LoginError:              "Произошла ошибка при входе. Пожалуйста, попробуйте снова",
			WrongPassword:           "Неверный пароль",
			UnknownCommand:          "Неизвестная команда. Используйте /start для получения списка доступных команд",
			GetMovieListError:       "Произошла ошибка при получении списка фильмов. Пожалуйста, попробуйте позже",
			InvalidID:               "Неверный ID",
			InvalidFileID:           "Неверный ID файла",
			MovieNotFound:           "Фильм не найден",
			GetFilesError:           "Ошибка при получении файлов для фильма",
			DeleteMovieDBError:      "Ошибка при удалении записи фильма из базы данных",
			DeleteFilesDBError:      "Ошибка при удалении файлов фильма из базы данных",
			InvalidCommandFormat:    "Неправильный формат команды",
			MovieListError:          "Ошибка при получении списка фильмов",
			MovieCheckError:         "Ошибка при проверке существования фильма",
			TorrentClientError:      "Не удалось создать торрент-клиент: %v",
			AddTorrentError:         "Не удалось добавить торрент: %v",
			NotEnoughSpace:          "Недостаточно места для загрузки фильма %s",
			GetMovieError:           "Ошибка при получении фильма: %v",
			DownloadStoppedLowSpeed: "Загрузка фильма %s остановлена из-за низкой скорости загрузки.",
			VideoTitleError:         "Ошибка получения названия видео: %s",
			VideoDownloadError:      "Ошибка загрузки видео: %s",
			VideoExists:             "Видео уже существует или в процессе загрузки",
		},
		Info: InfoMessages{
			VideoDownloading:            "Видео скачивается!",
			TorrentDownloadStarted:      "Загрузка торрента началась!",
			FileAlreadyExists:           "Файл уже существует",
			FileAddedSuccessfully:       "Файл успешно добавлен",
			StartCommand:                "<URL> - скачать потоковое видео\n/ls - получить список файлов\n/rm <ID> - удалить фильм, all - для удаления всех\n/stop - остановить все загрузки торрентов",
			TorrentDownloadsStopped:     "Все загрузки торрентов остановлены!",
			NoMovies:                    "Список пуст",
			LoginSuccess:                "Вход выполнен успешно",
			AllMoviesDeleted:            "Все видео удалены",
			MovieDeleted:                "Фильм успешно удален",
			DownloadVideoSuccess:        "Видео успешно скачано",
			MovieDownloaded:             "ID: %d\nНазвание: %s\nЗагружено: Да\n\n",
			MovieDownloading:            "ID: %d\nНазвание: %s\nПроцент загрузки: %d%%\n\n",
			StartDownload:               "Начата загрузка - %s",
			DownloadProgress:            "Загрузка %s: %d%%",
			DownloadComplete:            "Загрузка фильма %s завершена!",
			DownloadStopped:             "Загрузка фильма %s остановлена по запросу",
			StartVideoDownload:          "Начало загрузки для URL: %s",
			FileSavedAs:                 "Файл будет сохранён как: %s",
			VideoSuccessfullyDownloaded: "Видео успешно загружено: %s",
			UnknownUser:                 "Выполните вход с помощью команды /login [PASSWORD]",
		},
	},
}
