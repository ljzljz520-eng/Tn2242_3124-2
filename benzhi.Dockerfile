# Official Go image with the complete toolchain required by the evaluator.
FROM golang:1.22

WORKDIR /app

COPY ["go.mod","go.sum","/app/"]
RUN cd /app && GOWORK=off GOTOOLCHAIN=local go mod download

# Keep the complete project, including any project-owned Dockerfile and README.
COPY . .

RUN cd /app && GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off go build ./...

CMD ["bash"]
