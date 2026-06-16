ui = true

storage "file" {
  path = "/vault/data"
}

listener "tcp" {
  address       = "0.0.0.0:8200"
  tls_disable   = 1   # fine for local, use TLS for any real deployment
}

api_addr = "http://vault:8200"
disable_mlock = true
