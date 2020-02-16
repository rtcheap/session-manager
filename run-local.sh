cd cmd
go build
cd ..
mv cmd/cmd session-manager

export MIGRATIONS_PATH='./resources/db/mysql'
export JWT_ISSUER='rtcheap'
export JWT_SECRET='password'
export SERVICE_PORT='8082'

export SESSIONREGISTRY_URL='http://localhost:8080'
export SESSION_SECRET='super-secret-session-secret'
export TURN_UDP_PORT='3478'
export TURN_RPC_PROTOCOL='http'

export DB_HOST='127.0.0.1'
export DB_PORT='3306'
export DB_DATABASE='sessionmanager'
export DB_USERNAME='sessionmanager'
export DB_PASSWORD='password'

export JAEGER_SERVICE_NAME='session-manager'
export JAEGER_SAMPLER_TYPE='const'
export JAEGER_SAMPLER_PARAM=1
export JAEGER_REPORTER_LOG_SPANS='1'

./session-manager