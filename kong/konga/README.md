## Running Konga

### Development
```
$ npm start
```
Konga GUI will be available at `http://localhost:1337`

### Production

***************************************************************************************** 
In case of `MySQL` or `PostgresSQL` adapters, Konga will not perform db migrations when running in production mode.

You can manually perform the migrations by calling ```$ node ./bin/konga.js  prepare``` 
, passing the args needed for the database connectivity.

For example: 

```
$ node ./bin/konga.js  prepare --adapter postgres --uri postgresql://localhost:5432/konga
```
The process will exit after all migrations are completed. 

*****************************************************************************************

Finally:
```
$ npm run production
```
Konga GUI will be available at `http://localhost:1337`


### Production Docker Image

The following instructions assume that you have a running Kong instance following the
instructions from [Kong's docker hub](https://hub.docker.com/_/kong/)
```
$ docker pull pantsel/konga
$ docker run -p 1337:1337 \
             --network {{kong-network}} \ // optional
             --name konga \
             -e "NODE_ENV=production" \ // or "development" | defaults to 'development'
             -e "TOKEN_SECRET={{somerandomstring}}" \
             pantsel/konga
```

#### To use one of the supported databases

1. ##### Prepare the database
> **Note**: You can skip this step if using the `mongo` adapter.

You can prepare the database using an ephemeral container that runs the prepare command.

**Args**

argument  | description | default
----------|-------------|--------
-c      | command | -
-a      | adapter (can be `postgres` or `mysql`) | -
-u     | full database connection url | -

```
$ docker run --rm pantsel/konga:latest -c prepare -a {{adapter}} -u {{connection-uri}}
```


2. ##### Start Konga
```
$ docker run -p 1337:1337 
             --network {{kong-network}} \ // optional
             -e "TOKEN_SECRET={{somerandomstring}}" \
             -e "DB_ADAPTER=the-name-of-the-adapter" \ // 'mongo','postgres','sqlserver'  or 'mysql'
             -e "DB_HOST=your-db-hostname" \
             -e "DB_PORT=your-db-port" \ // Defaults to the default db port
             -e "DB_USER=your-db-user" \ // Omit if not relevant
             -e "DB_PASSWORD=your-db-password" \ // Omit if not relevant
             -e "DB_DATABASE=your-db-name" \ // Defaults to 'konga_database'
             -e "DB_PG_SCHEMA=my-schema"\ // Optionally define a schema when integrating with prostgres
             -e "NODE_ENV=production" \ // or 'development' | defaults to 'development'
             --name konga \
             pantsel/konga
             
             
 // Alternatively you can use the full connection string to connect to a database
 $ docker run -p 1337:1337 
              --network {{kong-network}} \ // optional
              -e "TOKEN_SECRET={{somerandomstring}}" \
              -e "DB_ADAPTER=the-name-of-the-adapter" \ // 'mongo','postgres','sqlserver'  or 'mysql'
              -e "DB_URI=full-connection-uri" \
              -e "NODE_ENV=production" \ // or 'development' | defaults to 'development'
              --name konga \
              pantsel/konga
```


The GUI will be available at `http://{your server's public ip}:1337`