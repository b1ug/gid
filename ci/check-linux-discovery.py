#!/usr/bin/env python3
"""Run the embedded hid_enumerate against fake libusb; never load real USB code.

Only the existing enumeration and free-list function bodies are compiled. All
libusb entry points, initialization, descriptors, strings and devices below are
software fixtures. The host platform need not be Linux. This proves that the
actual C VID/PID branch precedes string-query opens, not a native Linux result.
"""
import argparse
import json
from pathlib import Path
import subprocess
import tempfile


PREFIX = r'''
#include <assert.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <wchar.h>
#define HID_API_EXPORT
#define LIBUSB_CLASS_HID 3
struct hid_device_info {
 char *path; unsigned short vendor_id, product_id, release_number;
 wchar_t *serial_number, *manufacturer_string, *product_string;
 unsigned short usage_page, usage; int interface_number;
 struct hid_device_info *next;
};
struct libusb_device_descriptor {
 uint16_t idVendor, idProduct, bcdDevice;
 uint8_t iSerialNumber, iManufacturer, iProduct;
};
struct libusb_interface_descriptor { uint8_t bInterfaceClass, bInterfaceNumber; };
struct libusb_interface { const struct libusb_interface_descriptor *altsetting; int num_altsetting; };
struct libusb_config_descriptor { uint8_t bNumInterfaces; const struct libusb_interface *interface; };
typedef struct { unsigned index; } libusb_device;
typedef struct { unsigned index; } libusb_device_handle;
static void *usb_context;
static unsigned allocations, releases;
static void *tracked_calloc(size_t n, size_t size) { void *p=calloc(n,size); assert(p); allocations++; return p; }
static void tracked_free(void *p) { if(p)releases++; free(p); }
#define calloc tracked_calloc
#define free tracked_free
static const struct libusb_device_descriptor descriptions[] = {
 {0x27b8,0x01ed,0x204,1,2,3}, {1,2,1,1,2,3},
 {0,0x01ed,1,1,2,3}, {0x27b8,0,1,1,2,3},
 {0,0,1,1,2,3}, {0xffff,0xffff,1,1,2,3},
 {0x27b8,0x01ed,1,1,2,3}, {0x27b8,0x01ed,1,1,2,3}
};
#define COUNT (sizeof(descriptions)/sizeof(descriptions[0]))
static struct libusb_interface_descriptor altsettings[COUNT][2];
static struct libusb_interface interfaces[COUNT];
static struct libusb_config_descriptor configurations[COUNT];
static libusb_device devices[COUNT];
static libusb_device_handle handles[COUNT];
static libusb_device *device_list[COUNT+1];
static unsigned opened[COUNT], closed[COUNT], string_reads, config_frees, list_frees;
static int fail_init, fail_list, reject_open, fallback_config;
static unsigned init_calls;
static int hid_init(void) { init_calls++; return fail_init ? -1 : 0; }
static long libusb_get_device_list(void *ctx,libusb_device ***list) { (void)ctx; if(fail_list)return -1; *list=device_list;return COUNT; }
static int libusb_get_device_descriptor(libusb_device *dev,struct libusb_device_descriptor *desc) { *desc=descriptions[dev->index];return 0; }
static int libusb_get_active_config_descriptor(libusb_device *dev,struct libusb_config_descriptor **conf) { if(fallback_config)return -1; *conf=&configurations[dev->index];return 0; }
static int libusb_get_config_descriptor(libusb_device *dev,int index,struct libusb_config_descriptor **conf) { assert(index==0);*conf=&configurations[dev->index];return 0; }
static void libusb_free_config_descriptor(struct libusb_config_descriptor *conf) { assert(conf);config_frees++; }
static void libusb_free_device_list(libusb_device **list,int unref) { assert(list==device_list&&unref==1);list_frees++; }
static char *make_path(libusb_device *dev,int interface_num) { char *p=calloc(32,1);snprintf(p,32,"fixture-%u-%d",dev->index,interface_num);return p; }
static int libusb_open(libusb_device *dev,libusb_device_handle **handle) { opened[dev->index]++;if(reject_open)return -1;*handle=&handles[dev->index];return 0; }
static void libusb_close(libusb_device_handle *handle) { closed[handle->index]++; }
static wchar_t *get_usb_string(libusb_device_handle *handle,uint8_t index) {
 assert(handle->index<COUNT&&index>=1&&index<=3);string_reads++;
 wchar_t *p=calloc(2,sizeof(wchar_t));p[0]=L'A'+index;return p;
}
'''

SUFFIX = r'''
static int matches(unsigned i,unsigned short v,unsigned short p) {
 return (v==0||v==descriptions[i].idVendor)&&(p==0||p==descriptions[i].idProduct);
}
static void reset(void) {
 memset(opened,0,sizeof(opened));memset(closed,0,sizeof(closed));
 allocations=releases=string_reads=config_frees=list_frees=init_calls=0;
 fail_init=fail_list=reject_open=fallback_config=0;
 for(unsigned i=0;i<COUNT;i++) {
  devices[i].index=handles[i].index=i;device_list[i]=&devices[i];
  altsettings[i][0].bInterfaceClass=(i==6)?0:LIBUSB_CLASS_HID;
  altsettings[i][0].bInterfaceNumber=0;
  altsettings[i][1].bInterfaceClass=LIBUSB_CLASS_HID;
  altsettings[i][1].bInterfaceNumber=1;
  interfaces[i].altsetting=altsettings[i];interfaces[i].num_altsetting=(i==7)?2:1;
  configurations[i].bNumInterfaces=1;configurations[i].interface=&interfaces[i];
 }
 device_list[COUNT]=NULL;
}
static void run_case(unsigned short v,unsigned short p,int reject,int fallback) {
 reset();reject_open=reject;fallback_config=fallback;
 struct hid_device_info *head=hid_enumerate(v,p),*next=head;
 unsigned expected=0;
 for(unsigned i=0;i<COUNT;i++) {
  unsigned count=(i==6||!matches(i,v,p))?0:(i==7?2:1);
  assert(opened[i]==count);assert(closed[i]==(reject?0:count));
  for(unsigned n=0;n<count;n++) {
   char path[32];snprintf(path,sizeof(path),"fixture-%u-%u",i,n);
   assert(next&&next->vendor_id==descriptions[i].idVendor&&next->product_id==descriptions[i].idProduct);
   assert(strcmp(next->path,path)==0&&next->interface_number==(int)n);
   assert(next->release_number==descriptions[i].bcdDevice);
   assert(reject ? (!next->serial_number&&!next->manufacturer_string&&!next->product_string) : (next->serial_number&&next->manufacturer_string&&next->product_string));
   next=next->next;expected++;
  }
 }
 assert(!next&&string_reads==(reject?0:3*expected));
 assert(init_calls==1&&list_frees==1&&config_frees==COUNT);
 hid_free_enumeration(head);
 assert(allocations==releases);
}
int main(void) {
 const unsigned short pairs[][2]={{0x27b8,0x01ed},{0,0x01ed},{0x27b8,0},{0,0},{0xffff,0xffff},{0x1234,0x5678}};
 for(unsigned i=0;i<sizeof(pairs)/sizeof(pairs[0]);i++)run_case(pairs[i][0],pairs[i][1],0,0);
 run_case(0x27b8,0x01ed,1,0);run_case(0x27b8,0x01ed,0,1);
 reset();fail_init=1;assert(!hid_enumerate(0x27b8,0x01ed));assert(list_frees==0&&allocations==0);
 reset();fail_list=1;assert(!hid_enumerate(0x27b8,0x01ed));assert(list_frees==0&&allocations==0);
 puts("{\"status\":\"PASS\",\"cases\":10,\"host_usb_calls\":0,\"native_linux\":\"NOT_RUN\",\"boundary\":\"actual embedded hid_enumerate with fake libusb only\"}");
 return 0;
}
'''


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--cc', default='cc', help='C compiler executable, passed as argv without shell')
    args = parser.parse_args()
    root = Path(__file__).resolve().parents[1]
    header = (root / 'hidapi_linux.h').read_text()
    start = header.index('struct hid_device_info  HID_API_EXPORT *hid_enumerate(')
    end = header.index('\nhid_device * hid_open(', start)
    # Retain the actual default usage/detach macro section, too. Enabling the
    # invasive branch makes this fixture fail to compile, rather than silently
    # treating its unmodelled operations as safe metadata queries.
    macros_start = header.index('#ifndef __FreeBSD__\n#define DETACH_KERNEL_DRIVER')
    macros_end = header.index('/* Linked List of input reports', macros_start)
    source = PREFIX + header[macros_start:macros_end] + header[start:end] + SUFFIX
    with tempfile.TemporaryDirectory(prefix='gid-linux-discovery-c-') as temporary:
        path = Path(temporary)
        (path / 'fixture.c').write_text(source)
        subprocess.run([args.cc, '-std=c99', '-Wall', '-Wextra', '-Werror',
                        '-Wno-unused-but-set-variable', str(path / 'fixture.c'),
                        '-o', str(path / 'fixture')], check=True, timeout=30)
        result = subprocess.run([str(path / 'fixture')], check=True, timeout=10,
                                capture_output=True, text=True)
        report = json.loads(result.stdout)
        if report.get('status') != 'PASS' or report.get('host_usb_calls') != 0:
            raise RuntimeError('unexpected software fixture verdict')
        print(json.dumps(report, separators=(',', ':')))


if __name__ == '__main__':
    main()
