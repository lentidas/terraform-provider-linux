resource "linux_file" "file" {
  path         = "/tmp/linux/file"
  content      = "hello world\n"
  owner        = 1000
  group        = 1000
  mode         = "644"
  overwrite    = true
  recycle_path = "/tmp/recycle"
}
