resource "linux_directory" "directory" {
  path         = "/tmp/linux/directory"
  owner        = 1000
  group        = 1000
  mode         = "755"
  overwrite    = true
  recycle_path = "/tmp/recycle"
}
