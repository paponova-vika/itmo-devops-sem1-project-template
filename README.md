# Финальный проект 1 семестра

REST API сервис для загрузки и выгрузки данных о ценах.

## Требования к системе

- Операционная система: Linux (рекомендуется Ubuntu 20.04 и выше), Windows 10/11, MacOS.
- Язык программирования: Go версии 1.20 и выше.
- СУБД: PostgreSQL версии 12 и выше.

## Установка и запуск

```shell
git clone https://github.com/paponova-vika/itmo-devops-sem1-project-template.git
```
Установить зависимости, PostgreSQL, Go и настроить базу данных:

```shell
./scripts/prepare.sh
```

Запустить приложение: 

```shell
./scripts/run.sh
```

API доступно по адресу:

```shell
http://localhost:8080/api/v0/prices
```

## Тестирование

Директория `sample_data` - это пример директории, которая является разархивированной версией файла `sample_data.zip`

### Пример данных для загрузки:

Используйте файл sample_data.zip из директории sample_data для тестирования API.

### Тестовые запросы:

POST-запрос (загрузка данных):
```shell
curl -X POST -F "file=@sample_data.zip" http://localhost:8080/api/v0/prices
```
GET-запрос (выгрузка данных):

```shell
curl -X GET -o downloaded_data.zip http://localhost:8080/api/v0/prices
```
Скрипт тестирования: Запустите скрипт tests.sh, который выполнит тестовые запросы:

```shell
./scripts/tests.sh
```
### Результаты тестов: 
Убедитесь, что:
- POST-запрос возвращает JSON с количеством записей, категориями и общей стоимостью.
- GET-запрос возвращает ZIP-архив с файлом data.csv.

## Контакт

К кому можно обращаться в случае вопросов?
