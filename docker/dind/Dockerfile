FROM docker:dind

COPY start-dockerd.sh /usr/local/bin/start-dockerd.sh
RUN chmod +x /usr/local/bin/start-dockerd.sh

EXPOSE 2375

ENTRYPOINT ["start-dockerd.sh"]
