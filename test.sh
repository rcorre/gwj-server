#!/bin/sh -x
systemctl is-active docker >/dev/null || sudo systemctl start docker
POSTGRES_PASSWORD=abc123
CONTAINER_NAME=gwj-postgres
POSTGRES_PASSWORD=mysecretpassword
sudo docker run --name $CONTAINER_NAME -p 5432:5432 -e POSTGRES_PASSWORD=$POSTGRES_PASSWORD -d postgres:alpine
sleep 1
export DATABASE_URL="postgresql://postgres:$POSTGRES_PASSWORD@localhost:5432/postgres?sslmode=disable"
go test
sudo docker kill $CONTAINER_NAME >/dev/null
sudo docker container rm $CONTAINER_NAME >/dev/null
