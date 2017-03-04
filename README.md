bolt-server
-----------

Package bolt-server aims to implement a standards-compliant HTTP server on top
of BoltDB (http://github.com/boltdb/bolt). It supports HEAD, GET, PUT, and
DELETE verbs.

HEAD requests will retrieve the stored headers for a value, if they exist.

GET requests will either result in the retrieval of a stored value (and its
associated Content-Type, Content-Length and ETag) or a listing of a bucket's
contents (encoded as a JSON array). GET supports If-None-Match.

PUT requests with a body will create or overwrite a value. PUT requests without
a body will create a bucket if one does not already exist. If a PUT body is
non-empty, the size of the body must be specified with the Content-Length
header. If the client specifies the Content-Type header, the sever will store
it, and set it for subsequent GET requets. The server will compute the ETag
for the request body, and set the ETag header for subsequent GET requests.

DELETE requests will delete either a bucket or a value.

Example Usage
-------------

In this example, an instance of bolt-server is running on localhost:8080.
HTTP requests are sent to the server with curl. A web page is created, and
then retrieved.

```
$ curl -v http://localhost:8080
* Rebuilt URL to: http://localhost:8080/
*   Trying ::1...
* Connected to localhost (::1) port 8080 (#0)
> GET / HTTP/1.1
> Host: localhost:8080
> User-Agent: curl/7.43.0
> Accept: */*
>
< HTTP/1.1 200 OK
< Content-Type: application/json
< Date: Sat, 04 Mar 2017 05:54:18 GMT
< Content-Length: 3
<
[]

$ curl -v -X PUT -H 'Content-Type: text/html' -d '<html><body>Hello, world!</body></html>' http://localhost:8080/hello
*   Trying ::1...
* Connected to localhost (::1) port 8080 (#0)
> PUT /hello HTTP/1.1
> Host: localhost:8080
> User-Agent: curl/7.43.0
> Accept: */*
> Content-Type: text/html
> Content-Length: 39
>
* upload completely sent off: 39 out of 39 bytes
< HTTP/1.1 201 Created
< Etag: mHb90hhuT7g=
< Last-Modified: Sat, 04 Mar 2017 05:56:10 +0000
< Location: /hello
< Date: Sat, 04 Mar 2017 05:56:10 GMT
< Content-Length: 0
< Content-Type: text/plain; charset=utf-8
<

$ curl -v http://localhost:8080
* Rebuilt URL to: http://localhost:8080/
*   Trying ::1...
* Connected to localhost (::1) port 8080 (#0)
> GET / HTTP/1.1
> Host: localhost:8080
> User-Agent: curl/7.43.0
> Accept: */*
>
< HTTP/1.1 200 OK
< Content-Type: application/json
< Date: Sat, 04 Mar 2017 05:56:29 GMT
< Content-Length: 10
<
["hello"]

$ curl -v http://localhost:8080/hello
*   Trying ::1...
* Connected to localhost (::1) port 8080 (#0)
> GET /hello HTTP/1.1
> Host: localhost:8080
> User-Agent: curl/7.43.0
> Accept: */*
>
< HTTP/1.1 200 OK
< Content-Length: 39
< Content-Type: text/html
< Etag: mHb90hhuT7g=
< Last-Modified: Sat, 04 Mar 2017 05:56:10 +0000
< Date: Sat, 04 Mar 2017 05:56:39 GMT
<
<html><body>Hello, world!</body></html>

$ curl -v -H 'If-None-Match: mHb90hhuT7g=' http://localhost:8080/hello
*   Trying ::1...
* Connected to localhost (::1) port 8080 (#0)
> GET /hello HTTP/1.1
> Host: localhost:8080
> User-Agent: curl/7.43.0
> Accept: */*
> If-None-Match: mHb90hhuT7g=
>
< HTTP/1.1 304 Not Modified
< Etag: mHb90hhuT7g=
< Date: Sat, 04 Mar 2017 05:57:26 GMT
<

$ curl -v -X DELETE http://localhost:8080/hello
*   Trying ::1...
* Connected to localhost (::1) port 8080 (#0)
> DELETE /hello HTTP/1.1
> Host: localhost:8080
> User-Agent: curl/7.43.0
> Accept: */*
>
< HTTP/1.1 204 No Content
< Date: Sat, 04 Mar 2017 05:57:51 GMT
<
```
