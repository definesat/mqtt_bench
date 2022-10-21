FROM golang:1.18-buster
ADD bench /usr/local/bin/
RUN chmod +x /usr/local/bin/bench

ENTRYPOINT ["bench", "-h","define-iot.com","-c","50000"]
