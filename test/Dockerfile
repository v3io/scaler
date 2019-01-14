FROM golang:1.10

# download docker client to /usr/bin/docker
RUN curl https://download.docker.com/linux/static/stable/x86_64/docker-17.09.1-ce.tgz > /tmp/docker.tgz \
     && tar zxvf /tmp/docker.tgz --strip=1 -C /usr/bin docker/docker \
     && rm /tmp/docker.tgz
