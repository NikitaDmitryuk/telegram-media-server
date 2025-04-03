![build](https://img.shields.io/github/actions/workflow/status/NikitaDmitryuk/telegram-media-server/main.yml)
![downloads](https://img.shields.io/github/downloads/NikitaDmitryuk/telegram-media-server/total)
![release](https://img.shields.io/github/v/release/NikitaDmitryuk/telegram-media-server?display_name=tag)

# Telegram Media Server

Telegram Media Server is a Telegram bot that accepts links to streaming videos or torrent files, downloads them, and distributes them on the internal network via a DLNA server (e.g., `minidlna`).

Telegram Media Server — это Telegram-бот, который принимает ссылки на стриминговое видео или торрент-файлы, загружает их и раздает во внутренней сети через DLNA-сервер (например, `minidlna`).

---

## Contents / Оглавление

- [Features / Особенности](#features--особенности)
- [Installation / Установка](#installation--установка)
  - [Using Docker Compose / Использование Docker Compose](#using-docker-compose--использование-docker-compose)
  - [Installing the bot manually / Установка бота вручную](#installing-the-bot-manually--установка-бота-вручную)
  - [Installing and configuring minidlna / Установка и настройка minidlna](#installing-and-configuring-minidlna--установка-и-настройка-minidlna)
- [Configuration / Конфигурация](#configuration--конфигурация)
- [Usage / Использование](#usage--использование)
  - [Authorization / Авторизация](#authorization--авторизация)
  - [Available commands / Доступные команды](#available-commands--доступные-команды)
  - [Managing downloads / Управление загрузками](#managing-downloads--управление-загрузками)
  - [Examples of supported links / Примеры поддерживаемых ссылок](#examples-of-supported-links--примеры-поддерживаемых-ссылок)

---

## Features / Особенности

- **Receiving links / Прием ссылок**: Supports all video links supported by the `yt-dlp` utility. Поддерживает все ссылки на видео, которые поддерживаются утилитой `yt-dlp`.
- **Content Download / Загрузка контента**: Downloads videos and torrent files, tracking download progress. Загружает видео и торрент-файлы, отслеживая прогресс загрузки.
- **Distribution in internal network / Раздача во внутренней сети**: Distributes downloaded content via a DLNA server. Раздает загруженный контент через DLNA-сервер.
- **Download Management / Управление загрузками**: Allows you to view and manage current downloads via bot commands. Позволяет просматривать и управлять текущими загрузками через команды бота.
- **User Authorization / Авторизация пользователей**: Access to the bot is password protected. Доступ к боту защищен паролем.

---

## Installation / Установка

### Using Docker Compose / Использование Docker Compose

The easiest way to run Telegram Media Server is by using Docker Compose. This method works on any operating system and architecture that supports Docker.

Самый простой способ запустить Telegram Media Server — использовать Docker Compose. Этот метод работает на любой операционной системе и архитектуре, поддерживающей Docker.

1. **Clone the repository / Клонируйте репозиторий:**

   ```bash
   git clone https://github.com/NikitaDmitryuk/telegram-media-server.git
   cd telegram-media-server
   ```

2. **Create and edit the `.env` file / Создайте и отредактируйте файл `.env`:**

   ```bash
   cp .env.example .env
   ```

   Open the `.env` file and configure the parameters according to your requirements.

   Откройте файл `.env` и настройте параметры в соответствии с вашими требованиями.

3. **Start the container in the background / Запустите контейнер в фоне:**

   ```bash
   docker-compose up -d
   ```

4. **Check the logs / Проверьте логи:**

   ```bash
   docker-compose logs -f
   ```

5. **Stop the container / Остановите контейнер:**

   ```bash
   docker-compose down
   ```

---

### Installing the bot manually / Установка бота вручную

For Arch Linux users, there is an official package available. You can install it using the `pacman` package manager:

Для пользователей Arch Linux доступен официальный пакет. Вы можете установить его с помощью пакетного менеджера `pacman`:

```bash
sudo pacman -U telegram-media-server.pkg.tar.zst
```

Follow the steps in the [Configuration / Конфигурация](#configuration--конфигурация) section to set up the bot.

Следуйте шагам из раздела [Configuration / Конфигурация](#configuration--конфигурация), чтобы настроить бота.

---

### Installing and configuring minidlna / Установка и настройка minidlna

1. **Installing minidlna / Установка minidlna:**

   ```bash
   sudo apt install minidlna
   ```

2. **Configuring minidlna / Настройка minidlna:**

   Edit the configuration file **/etc/minidlna.conf** and configure the following parameters:

   Отредактируйте файл конфигурации **/etc/minidlna.conf** и настройте следующие параметры:

   ```conf
   media_dir=V,/path/to/dir
   friendly_name=My DLNA Server
   ```

   Replace **/path/to/dir** with the same path specified in the **MOVIE_PATH** parameter of the bot's **.env** file.

   Замените **/path/to/dir** на тот же путь, что указан в параметре **MOVIE_PATH** файла **.env** бота.

3. **Starting minidlna / Запуск minidlna:**

   ```bash
   sudo systemctl enable minidlna
   sudo systemctl start minidlna
   ```

---

## Configuration / Конфигурация

The bot configuration file is located at **/etc/telegram-media-server/.env**. Available parameters are described below:

Файл конфигурации бота находится по пути **/etc/telegram-media-server/.env**. Ниже описаны доступные параметры:

- `BOT_TOKEN (required / обязательно)`: Your Telegram bot token received from BotFather. Токен вашего Telegram-бота, полученный от BotFather.
- `MOVIE_PATH`: Path to the directory where the database, downloaded files, and movies will be stored. Путь к директории, где будут храниться база данных, загружаемые файлы и фильмы.
- `PASSWORD`: Password for authorizing users in the bot. Login is performed once for each chat. Пароль для авторизации пользователей в боте. Вход выполняется один раз для каждого чата.
- `LANG`: Bot message language. Supported values: ru, en. Язык сообщений бота. Поддерживаемые значения: ru, en.
- `PROXY`: Use proxy for yt-dlp. Proxy address. Использовать прокси для yt-dlp. Адрес прокси.
- `PROXY_HOST`: Use proxy only for listed domains. If empty, use proxy always. Использовать прокси только для перечисленных доменов. Если пустое, то использовать прокси всегда.

---

## Usage / Использование

### Authorization / Авторизация

Before using the bot, you must log in using the command:

Перед использованием бота необходимо авторизоваться с помощью команды:

```plaintext
/login <password>
```

Where **<password>** is the password specified in the **PASSWORD** parameter of the .env file.

Где **<password>** — пароль, указанный в параметре **PASSWORD** файла .env.

---

### Available commands / Доступные команды

- `/start` — Displays a welcome message. Отображает приветственное сообщение.
- `/login <password>` — User authorization in the bot. Авторизация пользователя в боте.
- `/ls` — Shows a list of current downloads and their status. Показывает список текущих загрузок и их статус.
- `/rm <id>` — Deletes a download by ID obtained from the /ls command. Удаляет загрузку по ID, полученному из команды /ls.
- `/rm all` — Deletes all current downloads. Удаляет все текущие загрузки.

---

### Managing downloads / Управление загрузками

After authorization, you can send the bot links to streaming videos or torrent files. The bot supports all links that are processed by the `yt-dlp` utility.

После авторизации вы можете отправлять боту ссылки на потоковые видео или торрент-файлы. Бот поддерживает все ссылки, которые обрабатываются утилитой `yt-dlp`.

<div style="display: flex; justify-content: space-between;">
  <img src="./images/manage_video.png" alt="Managing streaming videos" style="width: 45%;">
  <img src="./images/manage_torrent.png" alt="Managing torrent files" style="width: 45%;">
</div>

---

### Examples of supported links / Примеры поддерживаемых ссылок

- YouTube
- VK
- RuTube
- and others / и другие
