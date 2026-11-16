# Build context: an isolated directory containing static Linux parser + release/.
FROM scratch
COPY --chmod=0555 parser /parser
COPY release/ /opt/parser/release/
USER 10001:10001
ENV INFRAPRO_PARSER_ADDR=0.0.0.0:8091 INFRAPRO_PARSER_PACKAGE=/opt/parser/release INFRAPRO_PARSER_TOKEN_FILE=/run/secrets/parser-token
EXPOSE 8091
ENTRYPOINT ["/parser"]
