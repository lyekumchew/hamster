FROM golang:alpine as build
ADD . src
WORKDIR src
RUN GOOS=linux CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /src/hamster main.go

FROM alpine
COPY --from=build /src/hamster /hamster

EXPOSE 5050

ENV BASE=
ENV SECRET=
ENV ADDR=":5050"

ENTRYPOINT ["/bin/sh", "-c", "exec /hamster -secret ${SECRET} -base ${BASE} -addr ${ADDR}"]