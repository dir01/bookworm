#!/usr/bin/env python
import argparse

from settings import PROGRAM_NAME
from utils import print_execution_time
from webserver import webserver


def syncdb():
    from db.meta import get_session
    from settings import SQLALCHEMY_URL
    from db.models import Base
    session = get_session(SQLALCHEMY_URL)
    Base.metadata.create_all(session.bind)


@print_execution_time
def do_import(args):
    syncdb()
    from directory_importer import DirectoryImporter
    directory_importer = DirectoryImporter.get_instance()
    for directory in args.directories:
        directory_importer.do_import(directory)


def do_runserver(args):
    print 'Starting %s server at http://%s:%s' % (PROGRAM_NAME, args.host, args.port)
    webserver.serve(args.host, args.port)



parser = argparse.ArgumentParser(prog=PROGRAM_NAME)
subparsers = parser.add_subparsers()

parser_import = subparsers.add_parser('import', help='foo')
parser_import.add_argument('directories', type=str, nargs='+')
parser_import.set_defaults(func=do_import)

parser_runserver = subparsers.add_parser('runserver',
    help='Runs webserver with web representation of your library'
)
parser_runserver.add_argument('--host', default='localhost')
parser_runserver.add_argument('--port', type=int, default='4242')
parser_runserver.set_defaults(func=do_runserver)


if __name__ == '__main__':
    args = parser.parse_args()
    args.func(args)