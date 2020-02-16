NAME=`jq .name package.json -r`
VERSION=`jq .version package.json -r`
DB_NAME=${NAME//-}

docker run -d --name $NAME \
    --network rtcheap -p 8080:8080 \
    -e MIGRATIONS_PATH="/etc/$NAME/migrations" \
    -e JWT_ISSUER='rtcheap' \
    -e JWT_SECRET='password' \
    -e DB_HOST='rtcheap-db' \
    -e DB_PORT='3306' \
    -e DB_DATABASE=$DB_NAME \
    -e DB_USERNAME=$DB_NAME \
    -e DB_PASSWORD='password' \
    -e JAEGER_SERVICE_NAME=$NAME \
    -e JAEGER_SAMPLER_TYPE='const' \
    -e JAEGER_SAMPLER_PARAM=1 \
    -e JAEGER_REPORTER_LOG_SPANS='1' \
    -e JAEGER_AGENT_HOST='jaeger' \
    -e SESSION_SECRET='super-secret-session-secret' \
    eu.gcr.io/rtcheap/$NAME:$VERSION