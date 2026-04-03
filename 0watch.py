# Copyright (C) 2017, Bastian Eicher
# Edit by doudou0720,running in a temp dir and multiprocess

from __future__ import print_function

import argparse
import sys
import subprocess
import tempfile
import shutil
from os import path
from xml.dom import minidom
from zeroinstall.injector import model
import concurrent.futures

XMLNS_IFACE= 'http://zero-install.sourceforge.net/2004/injector/interface'

def die(msg):
    print(msg, file=sys.stderr)
    sys.exit(1)

def load(name, path):
    if sys.version_info >= (3,):
        from importlib.machinery import SourceFileLoader
        return SourceFileLoader(name, path).load_module()
    else:
        import imp
        return imp.load_source(name, path)

parser = argparse.ArgumentParser(description='Scan a website for new releases and trigger 0template if required.')
parser.add_argument('watch_file', help='Python script that pulls a list of releases from a website')
parser.add_argument('-o', '--output', help='output directory')
args = parser.parse_args()

watch_file = path.abspath(args.watch_file)
if not watch_file.endswith('.watch.py'):
    die("Watch file must be named *.watch.py, not '{watch_file}'".format(watch_file = watch_file))
if not path.exists(watch_file):
    die("Watch file '{watch_file}' must exist".format(watch_file = watch_file))
watch_file_stem = watch_file[:-9]

template_file = watch_file_stem + '.xml.template'
if not path.exists(template_file):
    die("Template file '{template_file}' must exist".format(template_file = template_file))

output_stem = watch_file_stem if args.output is None else path.join(args.output, path.basename(watch_file_stem))
feed_file = watch_file_stem + '.xml'
def output_file(version): return output_stem + '-' + model.format_version(version) + '.xml'

watch_module = load('watch', watch_file)
releases = getattr(watch_module, 'releases', None)
if not releases:
    die("Watch file must set array of dicts 'releases'")

def already_known(version):
    if path.exists(output_file(version)): return True
    if path.exists(feed_file):
        doc = minidom.parse(feed_file)
        for elem in doc.getElementsByTagNameNS(XMLNS_IFACE, 'implementation') + doc.getElementsByTagNameNS(XMLNS_IFACE, 'group'):
            v = elem.getAttribute('version')
            if v != '' and model.parse_version(v) == version: return True
    return False

def process_release(release):
    version = model.parse_version(release['version'])
    if already_known(version): return 0
    with tempfile.TemporaryDirectory(suffix='_'+str(version), prefix='0watch_') as dir_name:
        temp_template = shutil.copy(path.abspath(template_file),dir_name)
        retval = subprocess.call(['0template', '--output', path.abspath(output_file(version)), path.abspath(temp_template)] + [key + '=' + value for (key, value) in release.items()],cwd=dir_name)
        return retval

# Use ThreadPoolExecutor to process releases in parallel
with concurrent.futures.ThreadPoolExecutor() as executor:
    # Submit all release processing tasks
    future_to_release = {executor.submit(process_release, release): release for release in releases}
    
    # Process results as they complete
    for future in concurrent.futures.as_completed(future_to_release):
        release = future_to_release[future]
        try:
            retval = future.result()
            if retval != 0:
                print(f"Error processing release {release['version']}: returned {retval}", file=sys.stderr)
                sys.exit(retval)
        except Exception as exc:
            print(f"Error processing release {release['version']}: {exc}", file=sys.stderr)
            sys.exit(1)