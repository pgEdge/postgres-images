///////////////////////////
// pgedge-postgres image //
///////////////////////////

variable "PACKAGE_RELEASE_CHANNEL" {
  type    = string
}

variable "POSTGRES_MAJOR_VERSION" {
  type    = string
  default = ""
}

variable "TARGET" {
  type    = string
  default = ""
}

variable "PACKAGE_LIST_FILE" {
  type    = string
  default = ""
}

// Chained flavors need their own packagelist ARG. A flavor built FROM another
// flavor still triggers the parent stage, which consumes PACKAGE_LIST_FILE, so
// reusing that variable would make the parent install the child's list.
variable "COLDFRONT_PACKAGE_LIST_FILE" {
  type    = string
  default = ""
}

variable "TAG" {
  type    = string
  default = "pgedge"
}

target "default" {
  pull = true
  target = TARGET
  tags    = [TAG]
  args = {
    PACKAGE_RELEASE_CHANNEL     = PACKAGE_RELEASE_CHANNEL
    PACKAGE_LIST_FILE           = PACKAGE_LIST_FILE
    COLDFRONT_PACKAGE_LIST_FILE = COLDFRONT_PACKAGE_LIST_FILE
    POSTGRES_MAJOR_VERSION      = POSTGRES_MAJOR_VERSION
  }
  platforms = [
    "linux/amd64",
    "linux/arm64",
  ]
  attest = [
    "type=provenance,mode=min",
    "type=sbom",
  ]
}
