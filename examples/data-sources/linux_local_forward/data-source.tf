data "linux_local_forward" "remote" {
  remote_host = "127.0.0.1"
  remote_port = 3306
  local_host  = "127.0.0.1"
  local_port  = 3306
}
