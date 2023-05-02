FROM ubuntu:22.04
RUN apt-get update && apt-get upgrade -y

COPY ./mango /usr/bin/
ENTRYPOINT ["/usr/bin/mango"]
CMD ["--inventory.path", "/opt/mango/inventory"]
