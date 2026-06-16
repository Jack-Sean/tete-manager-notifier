FROM alpine:latest
RUN apk --no-cache add ca-certificates tzdata
ENV TZ=Asia/Shanghai
WORKDIR /root/
COPY tete-notifier .
CMD ["./tete-notifier"]
