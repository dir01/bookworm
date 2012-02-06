#!/usr/bin/env python
import argparse

from settings import PROGRAM_NAME
from utils import print_execution_time


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


parser = argparse.ArgumentParser(prog=PROGRAM_NAME)
subparsers = parser.add_subparsers()
parser_import = subparsers.add_parser('import', help='foo')
parser_import.add_argument('directories', type=str, nargs='+')
parser_import.set_defaults(func=do_import)


if __name__ == '__main__':
    args = parser.parse_args()
    args.func(args)