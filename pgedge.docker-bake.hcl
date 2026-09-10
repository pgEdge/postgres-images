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

// Every stage in the chain needs its own packagelist variable. A chained flavor
// still triggers its ancestors' stages, and each of those consumes its own ARG,
// so one shared variable would make an ancestor install a descendant's list.
variable "POSTGRES_PACKAGE_LIST_FILE" {
  type    = string
  default = ""
}

variable "STANDARD_PACKAGE_LIST_FILE" {
  type    = string
  default = ""
}

variable "COLDFRONT_PACKAGE_LIST_FILE" {
  type    = string
  default = ""
}

// Select what each chained stage is built FROM. Empty keeps the in-Dockerfile
// default (the parent stage), which is the single-graph build. A registry
// reference switches that stage to start from an already-published image, which
// is what a per-flavor CI wave needs.
variable "POSTGRES_IMAGE" {
  type    = string
  default = "postgres"
}

variable "MINIMAL_IMAGE" {
  type    = string
  default = "minimal"
}

variable "STANDARD_IMAGE" {
  type    = string
  default = "standard"
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
    POSTGRES_PACKAGE_LIST_FILE  = POSTGRES_PACKAGE_LIST_FILE
    PACKAGE_LIST_FILE           = PACKAGE_LIST_FILE
    STANDARD_PACKAGE_LIST_FILE  = STANDARD_PACKAGE_LIST_FILE
    COLDFRONT_PACKAGE_LIST_FILE = COLDFRONT_PACKAGE_LIST_FILE
    POSTGRES_IMAGE              = POSTGRES_IMAGE
    MINIMAL_IMAGE               = MINIMAL_IMAGE
    STANDARD_IMAGE              = STANDARD_IMAGE
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
