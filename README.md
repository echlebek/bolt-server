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
